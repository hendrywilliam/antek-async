# AGENTS.md

## Project State

`antek-async` is a Wails v2 desktop app that shows Kubernetes Nodes, Pods, Deployments and
StatefulSets. Pods and the workload controllers span **all namespaces**; nodes are cluster
scoped. It is deliberately small: read-only, no resource detail, no logs, no write operations,
no multi-cluster switching, and every list is filtered and grouped on the client. Each menu is
fetched from the cluster **only when it is opened**, so nothing is listed or watched in the
background. The one extra fetch is on demand: the pod table's actions dropdown can open a
read-only YAML view of that pod in a CodeMirror drawer.

## Stack / Toolchain

| Component | Version | Notes |
| --- | --- | --- |
| Go | 1.25.0 (declared in `go.mod`) | Module name is bare `antek-async`, not a URL path |
| Wails | v2.16.0 (`go.mod`) | CLI `wails` v2.16.0 must be on `PATH` (`~/go/bin/wails`) |
| client-go | v0.35.4 | Pinned deliberately, see the gotcha below |
| Node | v22 via nvm | Frontend deps live in `frontend/` |
| React | 19 | Function components only |
| Vite | 7 | Config at `frontend/vite.config.ts` |
| Tailwind | v4 via `@tailwindcss/vite` | No `tailwind.config.js`; the entry is `frontend/src/style.css` |
| shadcn/ui | CLI 4.x, `new-york` style | Config in `frontend/components.json`, components in `src/components/ui` |
| Inter | `@fontsource-variable/inter` | Bundled locally (no network), wired through `--font-sans` at 14px |
| CodeMirror | v6 + `@codemirror/lang-yaml` | Powers the read-only pod YAML drawer; adds ~300 kB to the bundle |

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
               ClientFor()      clientcmd -> kubernetes.Interface
nodes.go       NodeInfo         cluster scoped, no namespace
                                kubectl STATUS / ROLES / VERSION columns
pods.go        PodInfo / podStatus / WatchPods
                                flattened rows + port of kubectl's STATUS logic
deployments.go DeploymentInfo / WatchDeployments
statefulsets.go StatefulSetInfo / WatchStatefulSets
watch.go       Resource         the four watched kinds, plumbing only:
                                watchInformer[T], sort helpers, replicaCount

main  (Wails adapter)
─────────────────────────────────────────────────────────────────────────────
app.go   AppState + App      bound methods, per-resource state, watch lifecycle
         resourceHolder[T]  one per kind; only the open menu is streamed
main.go  wails.Run, Bind, OnStartup/OnShutdown
frontend/src/App.tsx          sidebar groups: Cluster > Nodes, Workloads > Pods,
                              Deployments, StatefulSets; Settings in the footer
                              one generic ResourceTable per kind, filter/Group By on the client
frontend/src/style.css        Tailwind v4 entry, dark-only black tokens, Inter at 14px
frontend/src/components/ui    shadcn components (table, sidebar, button, ...)
```

### Backend

- `internal/kube` is decoupled from Wails: each `WatchX` takes a `func([]X)` callback, so the
  whole cluster layer can be exercised by unit tests without a running app.
- **One resource kind per file**, and a watcher streams exactly one kind: `WatchNodes`,
  `WatchPods`, `WatchDeployments` and `WatchStatefulSets` each build their own single-informer
  factory. The shared informer, flush and ticker plumbing lives once in `watchInformer[T]`
  (`watch.go`), which is the only generic code in the package.
- **Nodes are cluster scoped**, which shows up in three places: the probe and informer take no
  namespace, `sortByName` replaces the namespace-then-name sort, and `NODE_ACCESSORS` in
  `App.tsx` omits `namespace` so the table hides the namespace filter and grouping.
- `PodYAML` renders one pod as YAML for the drawer. It is the only request that is not part of a
  watch, so `GetPodYAML` builds a short-lived client instead of reusing the active one, and it
  sets `apiVersion`/`kind` by hand (the typed client leaves them empty) and clears
  `managedFields` the way kubectl does by default.
- `Resolve` reads the environment and cwd, then delegates to `resolveKubeconfig`, which takes
  all four inputs as arguments. That split is what makes the precedence testable.
- `Resolve` picks exactly **one** kubeconfig file (no merging), so a project config never
  mixes with the home one.
- A watch treats the informer cache as the source of truth; no separate store exists. A `dirty`
  `atomic.Bool` set by the event handlers is swapped by a 1s flush ticker, so a busy cluster
  cannot flood the webview, and a second 10s ticker forces a republish so `Age` stays current.
  Before syncing it probes the resource with a `Limit: 1` list, so a missing RBAC rule surfaces
  as `cannot read <Kind>: ...` instead of a generic sync failure.
- `podStatus` ports the logic behind kubectl's STATUS column (init containers, then
  containers, then deletion state) and `Age` reuses `duration.HumanDuration`, the same helper
  kubectl uses. `deployments.go` and `statefulsets.go` follow the same idea for the READY /
  UP-TO-DATE / AVAILABLE counters kubectl prints.
- `app.go` keeps a `resourceHolder[T]` per kind behind one `sync.RWMutex` and exposes everything
  as a single `AppState`. `startWatch` is the only way a watcher starts, so opening a menu,
  switching kubeconfig and "retry" all share one code path.

### Backend to frontend contract

Four bound methods, all returning the same `AppState`: `GetState`, `SelectResource`,
`PickKubeconfig`, `ResetKubeconfig`. `AppState` carries the config plus one state object per
resource (`nodes`, `pods`, `deployments`, `statefulSets`), each holding `items`, `loaded`,
`loading`, `error` and `updatedAt`, so the frontend renders loading and failure per menu
without guessing. Every change is also pushed on the `state:update` event with an identical
payload, so the frontend has one reducer and never merges racing updates. The frontend calls
`GetState` on mount because events emitted before it subscribed are lost.

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
- **The app is dark only, and the palette is black, white and grey with one exception: status
  text.** `index.html` hard codes `class="dark"` on `<html>` and the `.dark` block in
  `src/style.css` is the active theme (pure black page, white text, lift comes from borders and
  greys); the light `:root` set is an unused fallback that exists because shadcn expects it.
  `main.go`'s window `BackgroundColour` must match the theme, since it shows before the webview
  paints. `StatusLabel` in `App.tsx` is the only place allowed to use colour, taken from
  Tailwind's default palette: `text-emerald-400` healthy, `text-amber-400` still starting,
  `text-red-400` for failures. Status is plain coloured **text**, not a badge, so do not
  reintroduce background or border chips. Do not add colour tokens to `style.css` for this.
- **Base font is Inter at 14px.** `src/main.tsx` imports `@fontsource-variable/inter` (bundled
  locally so the desktop app never needs the network) and `@theme inline` wires it through
  `--font-sans`, which Tailwind's preflight picks up via `--default-font-family`. `body` sets
  `font-size: 14px`, and markup relies on that inherited size instead of `text-xs`/`text-sm`
  overrides, so keep new text at the base size.
- **Only the open menu is fetched, and switching menus cancels the previous watch.**
  `SelectResource` bumps `App.generation`, cancels every running watcher and streams the
  requested kind, so `internal/kube` never has more than one watch open. Opening the active
  resource again is exactly the reload button, which is why there is no separate refresh method.
  Previously watched lists stay cached in their `resourceHolder`, so returning to a menu shows
  the old table immediately while it reconnects instead of flashing a spinner.
- **Changing kubeconfig drops every cached list.** `resetResourcesLocked` exists for this: the
  cached rows belong to the previous cluster, and showing them against a new kubeconfig would
  look like the new cluster's data.
- **The frontend calls `SelectResource` from click handlers, not from an effect keyed on the
  view.** React StrictMode invokes mount effects twice in dev, which would start the same watch
  twice. Go's `startup` activates pods so the first load needs no call from the frontend at all.
- **The payload deliberately avoids generics.** `PodsState`, `DeploymentsState` and
  `StatefulSetsState` are three concrete structs with the same shape because the Wails binding
  generator cannot name a generic instantiation. The generic `resourceHolder[T]` and
  `publishResource[T]`/`failResource[T]` helpers live on the Go side only, and `publishResource`
  is a free function because Go methods cannot take type parameters.
- **Every table's filter and Group By are client side.** The shared `ResourceTable` in
  `App.tsx` filters and buckets the in-memory rows and never asks the backend, so changing
  a filter cannot trigger cluster requests. Each view mounts its own `ResourceTable`, so
  switching sidebar entries starts from an unfiltered table, and a kind whose accessors omit
  `namespace` (nodes) renders no namespace filter or namespace grouping at all. Grouping renders
  an extra `TableRow` whose `colSpan` follows the column count plus the optional actions cell.
- **The pod YAML drawer lives outside the watch.** It fetches by namespace/name through
  `GetPodYAML`, so it works from any menu, and the row actions arrive through `ResourceTable`'s
  optional `rowActions` prop, which also widens the group header `colSpan`.
- **CodeMirror's `basicSetup` registers its default highlight style as a fallback**, which is why
  the monochrome `yamlHighlightStyle` in `App.tsx` wins without fighting it. Keep the editor
  colourless: the theme sets `{dark: true}` and uses the app's CSS variables. Note that the
  `codemirror` meta package does not re-export `EditorState`, and the read-only viewer needs only
  `EditorView.editable.of(false)`.
- **The YAML drawer opens from the right, and its width lives in `src/style.css`.** The Drawer
  caps the right direction at `sm:max-w-sm` (24rem) via
  `data-[vaul-drawer-direction=right]:sm:max-w-sm`, which compiles to the same specificity as any
  override passed at the call site, so source order decides and the component's rule wins. The
  unlayered `[data-slot="drawer-content"][data-vaul-drawer-direction="right"]` rule in
  `style.css` beats Tailwind's `utilities` layer, so the drawer gets 48rem and `drawer.tsx` stays
  pristine. A right drawer is full height, so the editor area is just `flex-1 min-h-0`.
- **The YAML drawer can only be closed with its own button, and that button must set the
  controlled state directly.** `dismissible={false}` makes vaul ignore overlay clicks, dragging
  and Escape, but it also makes vaul swallow its own close path: the `onOpenChange` handler it
  installs returns early when `open` is false, so a `DrawerClose` button would render and do
  nothing. The close button in the top right therefore calls `setYamlTarget(null)`, which flips
  the `open` prop itself. Never remove that button: between the disabled gestures and the
  swallowed close path, it is the only way out.
- **The kubeconfig details live in the Settings view**, reachable from the sidebar footer. The
  Settings view also lists, per resource kind, whether it has been loaded yet, how many items
  it holds and when it was refreshed. Resource views show no connection chrome: a list that has
  not loaded yet shows the spinner, and a failed load shows the error with its own retry.
  Sidebar selection is local `view` state, and there is no routing and no backend method behind
  Settings.
- **The shadcn registry installs `cn` as an npm package**, imported as `import { cn } from
  "cn"`. There is no `src/lib/utils.ts` even though the `aliases.utils` key exists in
  `components.json`; do not create one expecting components to use it.
- **`@import "tw-animate-css"` in `src/style.css` is required** for the `animate-in` /
  `animate-out` classes used by `sheet.tsx` and `tooltip.tsx`. Without it those classes are
  missing from the built CSS and the sidebar sheet and tooltips appear without animation.
- **The sticky table header depends on a CSS override** in `src/style.css`:
  `[data-slot="table-container"] { max-height: 100%; overflow-y: auto; }`. The `Table`
  component wraps the table in its own `overflow-x-auto` div, which would otherwise become
  the nearest scrollport and stop `sticky top-0` on `TableHeader` from working.
- `src/components/ui/*` and `src/hooks/use-mobile.ts` come from the shadcn registry. Re-adding
  a component overwrites it, so re-apply any local edits after `npx shadcn@latest add`.
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
  the adapter that holds state, emits events and opens dialogs. Every string the user can see
  (dialog titles, error messages, table text) is written in English, because it is displayed
  verbatim with no translation layer, so keep new user-facing strings English too.
- TypeScript: `strict` is on, `allowJs: false`, `noEmit: true`, `jsx: react-jsx`.
  `tsconfig.json` only includes `src/`, so `wailsjs/` itself is not type-checked.
- Frontend: `src/main.tsx` mounts `<App/>` into `#root` and imports the Tailwind entry
  `src/style.css`. Styling is Tailwind utility classes plus shadcn components from
  `@/components/ui`; there is no per-component CSS file. Backend calls are imported as named
  functions from `../wailsjs/go/main/App`, types from `../wailsjs/go/models`, events from
  `../wailsjs/runtime`.
- Add new UI primitives with the registry instead of hand-writing markup:
  `cd frontend && npx shadcn@latest add <component>`. Treat the generated files under
  `src/components/ui` as owned source that can be edited, and keep them monochrome.

## Testing

`go test ./...` runs pure unit tests in `internal/kube`, no cluster required:

- `kubeconfig_test.go` covers the resolution precedence, including that a higher priority
  source wins, that a vanished manual pick falls back to discovery, and that lower priority
  files are not chosen.
- `pods_test.go` covers the kubectl-equivalent status computation (init containers, waiting
  and terminated reasons, completed pods that are still running, deletion states), the
  Ready/Restarts/Age/Node flattening, and snapshot sorting.
- `deployments_test.go` and `statefulsets_test.go` cover the workload flattening: the READY
  count when `spec.replicas` is absent, the UP-TO-DATE and AVAILABLE counters, Age, and the
  sorting of each snapshot.
- `nodes_test.go` covers the node STATUS computation (Ready, NotReady, Unknown, a cordoned
  node and other conditions being ignored), the ROLES column (role-label prefix, legacy label,
  sorting and `<none>`), the VERSION/Age flattening, and name sorting.

There is no integration test infrastructure: `go test ./...` never talks to a cluster. A
throwaway test was used once to confirm that each watcher probes, syncs and publishes against
the real kubeconfig on this machine (pods, deployments and statefulsets all reached the
cluster), but it was deleted rather than checked in.
