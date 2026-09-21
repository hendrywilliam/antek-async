# AGENTS.md

## Project State

`antek-async` is a Wails v2 desktop app that shows a live list of Kubernetes Pods
across **all namespaces**. It is deliberately small: no filtering, no pod detail, no logs,
no write operations, no multi-cluster switching.

## Stack / Toolchain

| Component | Version | Notes |
| --- | --- | --- |
| Go | 1.25.0 (declared in `go.mod`) | Module name is bare `antek-async`, not a URL path |
| Wails | v2.16.0 (`go.mod`) | CLI `wails` v2.16.0 must be on `PATH` (`~/go/bin/wails`) |
| client-go | v0.35.4 | Pinned deliberately, see the gotcha below |
| Node | v22 via nvm | Frontend deps live in `frontend/` |
| React | 19 | Function components only |
| Vite | 7 | Config at `frontend/vite.config.ts` |

## Commands

Wails and Go commands run from the **repository root**; npm commands run from `frontend/`.

```bash
# Development (hot reload), also runs the Vite dev server.
wails dev                        # Browser devtools access at http://localhost:34115

# Production build -> build/bin/antek-async (gitignored)
wails build

# Tests, formatting, static checks
go test ./...
gofmt -l .                       # non-empty output means formatting is needed
go vet ./...

# Regenerate frontend/wailsjs bindings WITHOUT building or running the app.
wails generate module            # misleading name: it only regenerates bindings

# Frontend only
cd frontend && npm install       # frontend:install in wails.json
npm run dev                      # vite (frontend:dev:watcher)
npm run build                    # tsc && vite build
cd frontend && npx tsc --noEmit  # type-check alone; there is no lint script
```

## Architecture

```
internal/kube  (no Wails imports, unit-testable)
─────────────────────────────────────────────────────────────────────────────
kubeconfig.go  Resolve()        manual > $KUBECONFIG > ~/.kube/config
                                > <cwd>/.kube/config, one file, no merging
               Status/Candidate what the UI shows about the active cluster
               clientFor()      clientcmd -> kubernetes.Interface
pods.go        PodInfo          flattened table row (kubectl columns)
               podStatus()      port of kubectl's STATUS logic
               Watch(ctx, path, onSnapshot)  informer list+watch, coalesced

main  (Wails adapter)
─────────────────────────────────────────────────────────────────────────────
app.go   AppState + App      bound methods, state, watch lifecycle, events
main.go  wails.Run, Bind, OnStartup/OnShutdown
frontend/src/App.tsx          table + config header, subscribes to state:update
```

### Backend

- `internal/kube` is decoupled from Wails: `Watch` takes a `func([]PodInfo)` callback, so
  the whole cluster layer can be exercised by unit tests without a running app.
- `Resolve` reads the environment and cwd, then delegates to `resolveKubeconfig`, which takes
  all four inputs as arguments. That split is what makes the precedence testable.
- `Resolve` picks exactly **one** kubeconfig file (no merging), so a project config never
  mixes with the home one.
- `Watch` runs one informer per kubeconfig and treats the informer's own cache as the source
  of truth; no separate pod store exists. A `dirty` `atomic.Bool` set by the event handlers
  is swapped by a 1s flush ticker, so a busy cluster cannot flood the webview. A second 10s
  ticker forces a republish so the `Age` column does not go stale.
- `podStatus` ports the logic behind kubectl's STATUS column (init containers, then
  containers, then deletion state) and `Age` reuses `duration.HumanDuration`, the same helper
  kubectl uses.
- `app.go` holds all state behind one `sync.RWMutex` and exposes it as a single `AppState`.
  `restartWatch` is the only way a watcher starts, so switching kubeconfig and "retry" share
  one code path.

### Backend to frontend contract

Four bound methods, all returning the same `AppState`: `GetState`, `PickKubeconfig`,
`ResetKubeconfig`, `RefreshPods`. Every change is also pushed on the `state:update` event
with an identical payload, so the frontend has one reducer and never merges racing updates.
The frontend calls `GetState` on mount because events emitted before it subscribed are lost.

## Gotchas

- **Never edit `frontend/wailsjs/`.** It is generated (`DO NOT EDIT` header) and the whole
  `go/` directory is deleted and rewritten on every generation.
- **Types from another package become another TS namespace.** `AppState` lives in `main` and
  references `kube.Status` / `kube.PodInfo`, so `models.ts` contains both `export namespace
  main` and `export namespace kube`. The namespace name comes from the Go package name.
- **`wails build` and `wails dev` regenerate bindings** (unless `--skipbindings`), before
  compiling, so normally nothing extra is needed after adding a bound method.
- **Only exported methods on `*App` become JS bindings.** `startup` and `shutdown` are
  lowercase on purpose: they are lifecycle hooks wired via `OnStartup` / `OnShutdown` in
  `main.go`, not JS APIs.
- **`EventsEmit` only accepts the context from a lifecycle hook.** Passing nil, or any other
  context, makes the Wails runtime call `log.Fatalf`, which kills the process. This is why
  `emit` always uses `a.ctx` from `startup`.
- **`rest.Config.Timeout` must stay unset.** It applies to watches too and would tear down
  the pod watch on every interval. Bound individual requests with `context.WithTimeout`
  instead (see `probeTimeout`).
- **client-go is pinned to v0.35.4.** v0.36.2 and v0.37.0 declare `go 1.26.0`, which would
  bump the `go` directive in `go.mod`. v0.35.4 declares `go 1.25.0`.
- Adding client-go required network access for a few transitive modules that were missing
  from the local module cache; expect the first `go mod tidy` after a dependency change to
  download.
- **`frontend/dist` must exist for bare Go commands** (`go build`, `go test`, `go vet`,
  `gopls`) because of `//go:embed all:frontend/dist` in `main.go`. It is gitignored. `wails
  build` / `wails dev` create it themselves.
- **Cancelling the Wails file dialog returns an empty path with a nil error**, not an error.
  `PickKubeconfig` treats `""` as "no change". Also note `OpenDialogOptions.DefaultDirectory`
  must exist or the dialog call fails, which is why `kube.DefaultConfigDir` returns `""`
  rather than a missing path.
- **JSON struct tags decide the generated TypeScript field names.** A field without a `json`
  tag keeps its Go name in `models.ts`; all API types here use camelCase tags. Generated
  models are classes, but values arrive as plain JSON objects, so use them as types only.
- **React StrictMode mounts effects twice in dev.** `EventsOn` returns an unsubscribe
  function and `App.tsx` calls it in the effect cleanup; skipping that leaks a listener and
  duplicates every update.
- **Stale snapshots are dropped by an `App.generation` counter**, not by locking. A watcher
  that was superseded by a config switch can still be mid-callback, so `publish` and
  `setDisconnected` both ignore results whose generation is no longer current.
- **Unreachable clusters are handled, not retried forever**: the watch does one bounded
  `List` probe first (`probeTimeout`) so auth and connectivity errors surface with the API
  server's own message instead of a generic informer sync failure.
- Deployable assets live in `build/`: `darwin/Info*.plist`, `windows/*.manifest`,
  `windows/installer/*.nsi`, `build/appicon.png`. Deleting a platform directory and
  rebuilding restores Wails defaults.
- `frontend/package.json.md5` is the hash Wails uses to decide whether to re-run
  `frontend:install`. Do not hand-edit or delete it.

## Conventions

- Go: `gofmt` style, comments explain *why*, state guarded by a mutex is documented as such
  (for example `stateLocked` says callers must hold the lock). Keep new backend methods on
  `*App` so they are auto-bound.
- Keep cluster and kubeconfig logic in `internal/kube` and free of Wails imports; `main` is
  the adapter that holds state, emits events and opens dialogs. User-facing error strings in
  `internal/kube` are written in Indonesian because they are displayed verbatim.
- TypeScript: `strict` is on, `allowJs: false`, `noEmit: true`, `jsx: react-jsx`.
  `tsconfig.json` only includes `src/`, so `wailsjs/` itself is not type-checked.
- Frontend: `src/main.tsx` mounts `<App/>` into `#root`; plain CSS imported per component
  (`App.css`) plus global `style.css`. Backend calls are imported as named functions from
  `../wailsjs/go/main/App`, types from `../wailsjs/go/models`, events from
  `../wailsjs/runtime`.

## Testing

`go test ./...` runs pure unit tests in `internal/kube`, no cluster required:

- `kubeconfig_test.go` covers the resolution precedence, including that a higher priority
  source wins, that a vanished manual pick falls back to discovery, and that lower priority
  files are not chosen.
- `pods_test.go` covers the kubectl-equivalent status computation (init containers, waiting
  and terminated reasons, completed pods that are still running, deletion states), the
  Ready/Restarts/Age/Node flattening, and snapshot sorting.

There is no integration test infrastructure. Verifying real cluster behaviour means running
`wails dev` against a reachable kubeconfig; the checked-in dev machine's cluster was not
reachable, so that path was verified only through the unit tests above plus a bounded probe
against a real kubeconfig file.
