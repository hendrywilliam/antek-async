# AGENTS.md

## Project State

`antek-async` is a Wails v2 desktop app that shows Kubernetes Nodes, Namespaces, Pods,
Deployments, StatefulSets, Services and the four Gateway API kinds (GatewayClass, Gateway,
HTTPRoute and GRPCRoute). Pods, the workload controllers, services and the namespaced Gateway API
kinds span **all namespaces**; nodes, namespaces and gateway classes are cluster scoped. It is
deliberately small: no resource detail, no logs, no
multi-cluster switching, and
every list is filtered and grouped on the client. Each menu is fetched from the cluster **only
when it is opened**, so nothing is listed or watched in the background. The pod and node rows
also carry CPU and memory, which come from metrics-server and are read by the page that shows
them every five seconds while that page is open, because that API cannot be watched and has
nothing to push. Four things step outside
those lists: each table's actions dropdown opens a read-only YAML view of its row in a CodeMirror
drawer, and a pod's also opens an interactive **Terminal** in one of its containers, the
**Manifest YAML** menu writes, sending one hand-written manifest to the cluster with server-side
apply, and the **Namespaces** menu deletes a namespace once the user has typed the confirmation
word and the namespace name back.

## Stack / Toolchain

| Component | Version | Notes |
| --- | --- | --- |
| Go | 1.25.0 (declared in `go.mod`) | Module name is bare `antek-async`, not a URL path |
| Wails | v2.16.0 (`go.mod`) | CLI `wails` v2.16.0 must be on `PATH` (`~/go/bin/wails`) |
| client-go | v0.35.4 | Pinned deliberately, see the gotcha below |
| sigs.k8s.io/gateway-api | v1.4.0 | The Gateway API CRD types, clientset and informers. The four Gateway API menus are the only kinds this app does not get from client-go |
| Node | v22 via nvm | Frontend deps live in `frontend/` |
| React | 19 | Function components only |
| React Router | v7 (`react-router-dom`) | `HashRouter`, so every menu has its own hash route and the desktop build needs no server rewrites |
| Vite | 7 | Config at `frontend/vite.config.ts` |
| Tailwind | v4 via `@tailwindcss/vite` | No `tailwind.config.js`; the entry is `frontend/src/style.css` |
| shadcn/ui | CLI 4.x, `new-york` style | Config in `frontend/components.json`, components in `src/components/ui` |
| Inter | `@fontsource-variable/inter` | Bundled locally (no network), wired through `--font-sans` at 14px |
| JetBrains Mono | `@fontsource-variable/jetbrains-mono` | Bundled locally the same way, wired through `--font-mono`; a terminal cannot use Inter |
| xterm.js | `@xterm/xterm` v6 + `@xterm/addon-fit` | The interactive pod terminal's emulator and its fit-to-box addon |
| gorilla/websocket | v1.5.4-0.20250319132907-e064f32e3674 | The terminal's local listener; deliberately the version client-go already pins |
| CodeMirror | v6 + `@codemirror/lang-yaml` | Powers the read-only pod YAML drawer; adds ~300 kB to the bundle |

## Commands

**Do not run builds, tests or any other Go/Wails command yourself.** `go build`, `go test`,
`go vet`, `gofmt`, `go mod tidy`, `wails build` and `wails dev` all belong to the user, who runs
them and reports the result back. Write and edit the code, then hand off with the exact commands
to run instead of running them.

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
               RestConfigFor()  clientcmd -> rest.Config
               ClientFor()      rest.Config -> kubernetes.Interface
nodes.go       NodeInfo         cluster scoped, no namespace
                                kubectl STATUS / ROLES / VERSION columns
namespaces.go  NamespaceInfo / WatchNamespaces / DeleteNamespace
                                cluster scoped; kubectl NAME / STATUS / AGE columns;
                                the only delete path
pods.go        PodInfo / podStatus / WatchPods
                                flattened rows + port of kubectl's STATUS logic
deployments.go DeploymentInfo / WatchDeployments
statefulsets.go StatefulSetInfo / WatchStatefulSets
services.go    ServiceInfo / WatchServices
                                kubectl NAME / TYPE / CLUSTER-IP / EXTERNAL-IP / PORT(S)
                                / AGE columns
gatewayapi.go  conditionStatus / joinOrNone / gatewayAddresses / routeHostnames /
               gatewayAPIError  the helpers and the missing-CRD message the four
                                Gateway API kinds share
gatewayclasses.go GatewayClassInfo / WatchGatewayClasses
                                cluster scoped; kubectl NAME / CONTROLLER / ACCEPTED / AGE
gateways.go    GatewayInfo / WatchGateways
                                NAMESPACE / NAME / CLASS / ADDRESS / PROGRAMMED / AGE
httproutes.go  HTTPRouteInfo / WatchHTTPRoutes
                                NAMESPACE / NAME / HOSTNAMES / AGE
grpcroutes.go  GRPCRouteInfo / WatchGRPCRoutes
                                the same columns as HTTPRoute
metrics.go     PodUsage / NodeUsage / PodUsages, NodeUsages
                                CPU and memory from metrics.k8s.io, formatted the way
                                kubectl top prints them; one request, no polling
apply.go       ApplyResult / ApplyYAML
                                server-side apply for any discovered kind
terminal.go    TerminalRequest / PodContainers / ResolveContainer / ExecTerminal
                                one interactive session: the exec URL, the WebSocket
                                executor with SPDY behind it, and the size queue
watch.go       Resource         the watched kinds, plumbing only: watchInformer[T],
                                factoryStarter, sort helpers, replicaCount

main  (Wails adapter)
─────────────────────────────────────────────────────────────────────────────
app.go   AppState + App      bound methods, per-resource state, watch lifecycle
         resourceHolder[T]  one per kind; only the open menu is streamed
terminal.go terminalServer   the local WebSocket listener the terminal needs,
         terminalSession    the one-time tickets, and the frame protocol
main.go  wails.Run, Bind, OnStartup/OnShutdown
frontend/src/App.tsx          HashRouter shell: sidebar links, header, error banner
                              and Routes
frontend/src/routes.ts        route path/label/subtitle per menu; no cluster kind
frontend/src/app-context.tsx  AppContext: AppState plus busy/error/run/reload
frontend/src/use-resource.ts  per-page hook that starts the kind the page streams
frontend/src/pages            one page per menu: pods, deployments, statefulsets,
                              nodes, namespaces, services, gateway-classes, gateways,
                              http-routes, grpc-routes, manifest-yaml, settings
frontend/src/style.css        Tailwind v4 entry, dark-only black tokens, Inter at 14px
frontend/src/use-terminal.ts  the pod terminal's xterm instance and WebSocket
frontend/src/components/
  resource-table.tsx          generic table with filter/Group By on the client
  status-label.tsx            StatusLabel/NodeStatusLabel/NamespaceStatusLabel, the
                              only colour in the UI
  usage-cell.tsx              CPU and memory stacked in one cell, CPU on top
  terminal-drawer.tsx         drawer with the container picker that hosts the terminal
  yaml-style.ts               shared monochrome CodeMirror theme and highlight
  yaml-viewer.tsx             read-only CodeMirror YAML viewer
  yaml-editor.tsx             editable CodeMirror YAML editor
  yaml-drawer.tsx             the one YAML drawer; takes a noun and a per-kind fetcher
  delete-namespace-dialog.tsx two-input confirmation, then DeleteNamespace
  ui/                         shadcn components (table, sidebar, button, ...)
```

### Backend

- `internal/kube` is decoupled from Wails: each `WatchX` takes a `func([]X)` callback, so the
  whole cluster layer can be exercised by unit tests without a running app.
- **One resource kind per file**, and a watcher streams exactly one kind: `WatchNodes`,
  `WatchNamespaces`, `WatchPods`, `WatchDeployments`, `WatchStatefulSets`, `WatchServices` and the
  four Gateway API watchers
  each build their
  own single-informer factory. The shared informer, flush and ticker plumbing lives once in
  `watchInformer[T]` (`watch.go`), which is the only generic code in the package. Its `factory`
  parameter is the one-method `factoryStarter` interface rather than a concrete factory type,
  because the Gateway API factory is a different interface that shares only `Start`.
- **Gateway API is a set of CRDs, so it is the one kind group that does not come from client-go.**
  The typed types, clientset and informers come from the `sigs.k8s.io/gateway-api` module, which
  is why `GatewayClientFor` (`kubeconfig.go`) sits beside `ClientFor` and `watchGatewayAPI`
  (`app.go`) builds that client instead of the core one. A single `GatewayV1()` clientset serves
  all four kinds: typed `List` probes, typed informers through `factory.Gateway().V1()`, and typed
  field reads, so each file has the shape of `services.go` and the tables are built from
  `GatewayClass.Spec.ControllerName`, `Gateway.Spec.GatewayClassName`,
  `Gateway.Status.Addresses`, `HTTPRoute.Spec.Hostnames` and `GRPCRoute.Spec.Hostnames`.
- The four Gateway API columns are ports of the CRDs' own `additionalPrinterColumns`, which is
  why addresses and hostnames join the list **in stored order** rather than sorting it: the API
  server renders those jsonPaths as-is, unlike the service load balancer addresses that
  `services.go` does sort. ACCEPTED and PROGRAMMED come from `status.conditions`, and a kind whose
  controller has not been reconciled yet has no such condition at all, which reads as `<none>`.
- **The two route tables add one column kubectl has not got: PARENT REFS.** It comes from
  `spec.parentRefs` through `routeParentRefs`, which resolves a reference without a namespace to
  the route's own namespace and keeps a listener section when one is named. That resolution is what
  makes the value usable as a Group By key: a bare name would otherwise put every namespace's
  `my-gateway` into one bucket.
- **A cluster without the CRDs is the expected case, not a fault in the request.** The probe turns
  the API server's 404 into "the Gateway API CRDs are not installed" (`gatewayAPIError`), because
  "the server could not find the requested resource" never says Gateway API. The four menu entries
  are always listed, and each page reports that message with its own Retry.
- **Nodes, namespaces and gateway classes are cluster scoped**, which shows up in three places:
  the probe and informer take no namespace, `sortByName` replaces the namespace-then-name sort, and
  the accessors (`NODE_ACCESSORS` in `src/pages/nodes.tsx`, `NAMESPACE_ACCESSORS` in
  `src/pages/namespaces.tsx`, `GATEWAY_CLASS_ACCESSORS` in `src/pages/gateway-classes.tsx`) omit
  `namespace`, so the table hides the namespace filter and
  grouping.
- `PodYAML` renders one pod as YAML for the drawer. Like `ApplyYAML`, it is not part of a watch,
  so `GetPodYAML` builds a short-lived client instead of reusing the active one, and it sets
  `apiVersion`/`kind` by hand (the typed client leaves them empty) and clears `managedFields`
  the way kubectl does by default.
- The Gateway API kinds get the same treatment, one `GatewayClassYAML`, `GatewayYAML`,
  `HTTPRouteYAML` and `GRPCRouteYAML` per kind file, all mirroring `PodYAML` down to the hand-set
  `apiVersion` (from `gatewayAPIGroupVersion`) and the dropped `managedFields`. `App.gatewayClient`
  is what builds the client for those four on-demand reads.
- `ApplyYAML` in `apply.go` is the write path for creating and updating. It decodes one YAML or
  JSON document into an `unstructured.Unstructured`, resolves the kind through discovery and
  `restmapper` so any kind
  the cluster knows works, and sends it as a server-side apply `PATCH`. `Force` stays off, so a
  field another manager owns surfaces as a conflict instead of being taken over.
- `DeleteNamespace` in `namespaces.go` is the only path that removes anything, and it deletes
  exactly one namespace, named by the caller. Finalizers are left to the API server, so a
  namespace that takes time to go drains in the watch as Terminating before it disappears. The
  name is what the frontend makes the user type back before the call is sent; the backend only
  refuses an empty one.
- **The pod terminal is the only interactive, long-lived connection in the app.** `ExecTerminal`
  in `terminal.go` runs `exec` (not `attach`) in one container with a TTY, so it works on any pod
  whose image carries a shell; `/bin/sh` is the default because it is the one shell every
  non-distroless image has. It builds the executor `kubectl exec` builds, a WebSocket upgrade
  with the SPDY protocol behind it, and only an upgrade failure falls back, so an error from
  inside the container still reaches the drawer instead of being retried on the other protocol.
  Like `ApplyYAML` it takes a kubeconfig path, because the executor needs the `*rest.Config` as
  well as the REST client that builds the exec URL. It takes a `TermStreams` (stdin, stdout and a
  channel of sizes) rather than a socket, which is what keeps this package free of the transport
  and exercisable without a cluster.
- `PodContainers` lists what a terminal can target in one pod and deliberately leaves
  `initContainers` out, because they run to completion before the pod is up. `ResolveContainer`
  defaults an empty request to the first container and refuses a name the pod does not declare,
  where the message can say which container is missing instead of letting an exec request fail on
  the wire.
- **`ExecTerminal` must never pick up `requestTimeout`.** A terminal lives as long as the drawer
  is open, so its session is bounded by its context alone; `rest.Config.Timeout` is left unset the
  way `RestConfigFor` leaves it. `PodContainers`, being one bounded read, does use
  `requestTimeout` like `PodYAML`.
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
- **CPU and memory are not part of a watch.** Metrics come from the metrics-server API, which
  has no watch and pushes nothing, so `metrics.go` offers one request per kind: `PodUsages` and
  `NodeUsages`, reached through the two bound methods `GetPodUsages` / `GetNodeUsages`. Who
  decides how often to ask are the two pages that show the numbers, not this package. The
  numbers are formatted by
  `formatCPU` and `formatMemory`, which are the helpers behind kubectl's CPU(cores) and
  MEMORY(bytes) columns (millicores, and memory in mebibytes), and a pod's containers are summed
  the way `kubectl top pod` sums them.
- The metrics API is called through the discovery REST client with an `AbsPath` and decoded into
  a hand-written subset of `metrics.k8s.io/v1beta1`, which keeps the package off the
  `k8s.io/metrics` module: that module would have to be pinned to the client-go version for no
  gain, since two endpoints and four fields are all the tables need.
- `app.go` keeps a `resourceHolder[T]` per kind behind one `sync.RWMutex` and exposes everything
  as a single `AppState`. `startWatch` is the only way a watcher starts, so opening a menu,
  switching kubeconfig and "retry" all share one code path.
- **The terminal's transport is a loopback WebSocket server, because Wails cannot serve one.**
  The asset server is not a real TCP listener and its `http.ResponseWriter` is not an
  `http.Hijacker`, so a WebSocket upgrade cannot pass through it; `terminal.go` in `main`
  therefore owns a `net.Listen` on `127.0.0.1:0`. Binding port zero lets the kernel pick a free
  port, and the listener is created lazily on the first `OpenTerminal`, so a user who never opens
  a terminal never has a port open. `shutdown` closes it.
- **A ticket, not the port, is what protects a session.** Every local process can reach loopback
  and a session runs a shell with the user's credentials, so `OpenTerminal` mints a single-use
  32-byte token with a 30s TTL and only a connection that spends it gets a session; stale tickets
  are swept when a new one is issued rather than on a timer. `CheckOrigin` is set as well, but
  gorilla's default could never work here: it compares the Origin host against the request Host,
  and the webview reports a `wails://` origin while the request goes to `127.0.0.1`, so the
  default would refuse every handshake.
- A ticket captures the kubeconfig path, so a session stays on the cluster it was prepared
  against, and a kubeconfig switch ends every live session: a shell belongs to the cluster it was
  started on.

### Backend to frontend contract

`GetState`, `SelectResource`, `PickKubeconfig` and `ResetKubeconfig` all return the same
`AppState`; `GetPodYAML`, `GetPodContainers`, `OpenTerminal`, `ApplyYAML`, `DeleteNamespace`,
`GetPodUsages`, `GetNodeUsages` and the four Gateway API YAML readers (`GetGatewayClassYAML`,
`GetGatewayYAML`, `GetHTTPRouteYAML`, `GetGRPCRouteYAML`) are
the on-demand requests
outside the watch and return their own types, and each of them reports its own failure to the
page that asked instead of through `AppState`. `AppState` carries the config plus one state object per resource
(`nodes`, `namespaces`, `pods`, `deployments`, `statefulSets`, `services`, `gatewayClasses`,
`gateways`, `httpRoutes`, `grpcRoutes`), each holding `items`,
`loaded`,
`loading`, `error` and `updatedAt`, so the frontend renders loading and failure per menu without
guessing; CPU and memory are deliberately absent from it, because they belong to whoever polls
them. Every change is
also pushed on the `state:update` event with an identical
payload, so the frontend has one reducer and never merges racing updates. The frontend calls
`GetState` on mount because events emitted before it subscribed are lost.

The terminal is the one request that is not a single call. `GetPodContainers` and `OpenTerminal`
are bound methods like the rest, but `OpenTerminal` only validates the request against the pod
and returns a one-time endpoint; the session itself is a WebSocket at that endpoint, with a small
protocol of its own. Output and typed input travel as **binary** frames, because a read can split
a multi-byte character and only raw bytes survive that intact. The two control messages the
drawer sends (a resize, and a request to end the session) and the two it receives (`ready`, then
`exit` with a code and the cluster's own message) are JSON in **text** frames, so the frame type
is the whole discriminator. Nothing about a session reaches `AppState` or `state:update`.

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
- **gateway-api is pinned to a release, never to `main`.** `sigs.k8s.io/gateway-api` `main`
  declares `go 1.26.0`, which would bump the `go` directive and need a newer toolchain, while every
  release up to v1.4.0 declares `go 1.24.0` or lower. v1.4.0 therefore keeps `go 1.25.0` intact,
  and it requires client-go v0.34.1, which is below this app's v0.35.4 pin, so MVS keeps ours. The
  module is not in the local module cache, so the first `go mod tidy` after a change here needs
  network.
- **The four Gateway API menus target `v1` only.** GatewayClass, Gateway and HTTPRoute have been
  served as `v1` since Gateway API v1.0.0, but GRPCRoute only since v1.2.0, so an older install
  reports the not-installed message for that one kind instead of the app falling back to
  `v1beta1`. The typed clientset does expose `GatewayV1beta1()`, which is the hook for adding that
  fallback if it is ever wanted.
- **The Gateway API kinds need a second clientset, so `streamResource` builds one for them.** The
  core client is still built at the top of that function and goes unused on the gateway path,
  which costs one kubeconfig read per menu open and is deliberate: it keeps the six existing kinds
  on exactly the code path they had.
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
  paints. `StatusLabel` in `components/status-label.tsx` is the only place allowed to use colour,
  taken from
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
- **Each menu is its own hash route, and the page owns the kind it streams.** `src/routes.ts`
  holds only navigation metadata (path, label, subtitle), `App.tsx` wraps the shell in
  `HashRouter` and drives the sidebar through `react-router-dom` links, and every page under
  `src/pages` is a separate component. `HashRouter` rather than `BrowserRouter` is deliberate:
  the desktop build serves the frontend with no server that could answer a path-based route.
- **The Gateway API children are a collapsible submenu, not four more sidebar groups.**
  `MENU_GROUPS` in `App.tsx` holds `MenuEntry` values, which are either a leaf `{ view }` or a
  `{ label, icon, views }` submenu, and a submenu renders through the `SidebarMenuSub` primitives
  with a `defaultOpen` computed from the route on screen, so landing on a child never hides the
  link that is active. `Collapsible` comes from the already-installed unified `radix-ui` package
  (`ui/collapsible.tsx`), so no npm dependency was added. The children deliberately carry no icon,
  which is why `VIEW_ICONS` is a `Partial<Record<View, …>>`: a full record would have forced four
  invented icons for indented rows.
- **The gRPC route menu is labelled `GRPCRoute`**, the kubectl kind name, matching the other three
  rather than the "gRPC Route" prose spelling.
- **A page starts its own watch through `useResource` (`src/use-resource.ts`), so `App.tsx`
  never names a resource.** The hook calls `SelectResource` when the page mounts and keeps the
  ref guard that stops StrictMode's double mount from cancelling and restarting the same watch.
  It also lends the shell its reconnect function, so the header's Reload button stays generic
  and re-clicking the open menu in the sidebar reloads through the same path. Pages read the
  shared `AppState` from `AppContext`; Settings streams nothing and uses `useApp` directly.
  Go's `startup` still warms the pods watch, so the first route usually renders rows before the
  webview paints, at the cost of one extra list when the pods page reconnects it.
- **The payload deliberately avoids generics.** Every kind's state is its own concrete struct with
the same shape (`PodsState`, `ServicesState`, `GatewayClassesState`, …) because the Wails binding
generator cannot name a generic instantiation. The generic `resourceHolder[T]` and
  `publishResource[T]`/`failResource[T]` helpers live on the Go side only, and `publishResource`
  is a free function because Go methods cannot take type parameters.
- **Every table's filter and Group By are client side.** The shared `ResourceTable` in
  `components/resource-table.tsx` filters and buckets the in-memory rows and never asks the
  backend, so changing a filter cannot trigger cluster requests. Each page mounts its own
  `ResourceTable`, so switching sidebar entries starts from an unfiltered table, and a kind whose
  accessors omit `namespace` (nodes, namespaces, gateway classes) renders no namespace filter or
  namespace
  grouping at all. Grouping renders
  an extra `TableRow` whose `colSpan` follows the column count plus the optional selection and
  actions cells.
- **Row selection is opt-in on the shared table.** A list that offers a bulk action passes
  `selectable` and a `toolbar`, which `ResourceTable` renders above the table and calls with the
  ticked rows; today only the namespaces page does, because deleting is the only bulk action.
  The ticks are stored as row keys and re-derived from the rows on every render, so a namespace
  that a delete removed cannot stay ticked, and the header checkbox covers the rows the filter
  currently shows rather than every row in the cluster.
- **The CPU and memory column is an overlay, not a row field.** The pods and nodes pages add one
  `CPU / Memory` column of their own, spliced in right after `Ready` (pods) and after `Status`
  (nodes, which has no Ready column) by header name rather than by index. It is built with
  `useMemo` from the page's own `usage` state, because module-level column arrays
  cannot see a per-render value, and the cell (`components/usage-cell.tsx`) stacks CPU over
  memory from the entry whose key matches the row, or `-` when the cluster has no metrics for it.
  That is why those two pages pass `notice` to `ResourceTable`: the page's `usageError` explains
  the empty
  column instead of taking over a cell, and the other pages leave the prop at its empty default.
  Nothing here is per row state, so a refresh cannot disturb a filter or a tick.
- **The metrics interval lives in the two pages that show the numbers.** `pods.tsx` and
  `nodes.tsx` each run a `useEffect` that asks once, then on `USAGE_INTERVAL` (5s) until the page
  unmounts; there is no hook in between, and nothing about the window changes the cadence.
  Pausing it while the window was unfocused was tried twice and removed again, in a hook and then
  in the page: it hung the refresh on focus and visibility signals that an embedded webview does
  not deliver or answer reliably, so a signal that never arrived left the column frozen until the
  menu was reopened. The only things that stop the poll are the ones that cannot fail to happen:
  leaving the menu (unmount) and switching kubeconfig, which also drops the numbers belonging to
  the previous cluster.
- **The usage request is awaited inside a try, and that is what makes a failure visible.**
  `GetPodUsages` / `GetNodeUsages` have to be able to fail loudly: a binding that is missing or a
  cluster that refuses the call used to leave the column empty with no explanation, because a
  synchronous throw skipped the promise chain and the state was never updated. Now any throw or
  rejection lands in the page's own `usageError` and shows up as the table's `notice`. A tick
  that lands while a request is still out is skipped, so a slow API server cannot stack requests,
  and a failed ask empties the values, because stale numbers would read as current.
- **The usage state belongs to the page, and to one kubeconfig.** The effects depend on
  `state.config.path`, so a new kubeconfig clears the numbers instead of showing them against the
  new cluster's rows. That state never goes through the shell's `run`, which means the poll cannot
  flip the shared `busy` flag and disable the header buttons every five seconds. Unmounting the
  page clears the interval, so switching menus stops the polling.
- **The namespace delete is confirmed twice before it is sent.** The toolbar only queues the
  ticked namespaces; `delete-namespace-dialog.tsx` is then mounted once per namespace with a
  `key`, so its two inputs (the word `delete`, and the namespace name typed back) start empty
  every time. Confirm checks both, refuses to send anything on a mismatch, and only then calls
  `DeleteNamespace`. A multiple selection is walked one namespace at a time, Cancel drops the
  rest of the queue, and the dialog reports a rejected delete in line instead of through the
  shared error banner. Deleting does not refresh the list: the watch delivers the namespace as
  Terminating and then as gone.
- **The YAML drawer lives outside the watch and serves every kind.** `YamlDrawer
  (frontend/src/components/yaml-drawer.tsx)` takes a `target`, a `noun` and a `fetchYaml` callback,
  and only the name has to be set, so a cluster-scoped kind hands over an empty namespace and the
  description drops it. The fetcher is a prop because the drawer is shared, so pages pass a
  module-level function (or a binding directly) and its identity stays stable across renders: an
  inline arrow would re-run the effect and refetch on every render. The row actions arrive through
  `ResourceTable`'s optional `rowActions` prop, which also widens the group header `colSpan`, and
  each page builds its own dropdown in the page, the way the pods table does.
- **The terminal's WebSocket cannot be served by the Wails asset server**, so it is a separate
  loopback listener. Two things are worth knowing before debugging one that will not open: the
  page must be allowed to reach `ws://127.0.0.1` (loopback is normally exempt from ATS, but a
  production `wails://` page is the case to check first), and a refused handshake is logged with
  the observed `Origin` header, which is the only way to confirm the allowlist because each
  platform reports a different origin.
- **The terminal drawer's host element is state, not a ref object.** `useTerminal` takes the
  element itself, and the drawer hands it over through `ref={setHost}`, so the hook re-runs when
  the element appears. A `useRef` would be null on the first render, and the effect would return
  early and never run again, which fails silently: the drawer opens on an empty box.
- **`ws.binaryType = "arraybuffer"` is required on the client.** Without it the browser hands back
  `Blob` objects, every chunk of output is dropped, and it looks like a terminal that connects and
  then does nothing.
- **The xterm theme uses literal hex, not the `style.css` tokens**, because xterm's colour parser
  cannot read `oklch()`. The app is dark only, so the terminal carries its own grey ramp, and the
  ANSI entries are deliberately part of it: a shell's colours are flattened to greys to keep the
  terminal inside the app's black, white and grey rule.
- **A session the user closed is not a failure.** The stream reports its context error as the
  session's result, so `terminalSession.end` reports a clean exit whenever the session's context is
  already done, and only carries an exit code or a reason otherwise.
- **gorilla/websocket is pinned by client-go, not by this app.** It is imported directly for the
  listener, but the version must stay `v1.5.4-0.20250319132907-e064f32e3674`, which is the one
  `client-go` v0.35.4 already resolves. Pulling in the exec and SPDY packages also added
  `github.com/moby/spdystream` and `github.com/mxk/go-flowrate` to `go.sum`; they were missing from
  the cache and `go mod tidy` needed them, so expect the same after any change that touches the
  exec path.
- **CodeMirror's `basicSetup` registers its default highlight style as a fallback**, which is why
  the monochrome `yamlHighlightStyle` in `yaml-viewer.tsx` wins without fighting it. Keep the editor
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
  nothing. The close button in the top right therefore calls the drawer's `onClose` prop, which
  the pod page wires to `setYamlTarget(null)` so it flips the `open` prop itself. Never remove that
  button: between the disabled gestures and the swallowed close path, it is the only way out.
- **The Manifest YAML menu is the only path that creates or updates, and it is deliberately
  hard to misuse.** `ApplyYAML` sends exactly one document with server-side apply and `Force`
  off, so a
  field owned by another manager surfaces as a conflict instead of being taken over. It refuses
  a manifest without
  `apiVersion`, `kind` or `metadata.name`, and it refuses more than one document rather than
  silently applying the first. A namespaced manifest without `metadata.namespace` lands in
  `default`, matching kubectl (`placement`), while a cluster-scoped kind has its namespace
  stripped. There is no scale and no dry-run. The page reports its own outcome: the
  error goes inline in `text-red-400`, the same colour the YAML drawer uses, and a success is
  plain text, so the shared error banner stays reserved for configuration problems.
- **The editor must not be rebuilt on every keystroke.** `YamlEditor`
  (`components/yaml-editor.tsx`) creates its `EditorView` once per seed document and reports
  changes through `EditorView.updateListener`; handing the live value back as `initialDocument`
  would reset the cursor and the undo history, so clearing the page remounts the editor by
  bumping `key={seed}` instead. `components/yaml-style.ts` holds the monochrome theme and
  highlight style that the editor and the viewer share, so the two never drift apart. Only the
  viewer disables editing, because CodeMirror's `basicSetup` is editable by default; the editor
  therefore needed no new npm packages.
- **The manifest page streams no kube kind but still has a backend method.** Like Settings it uses
  `useApp` rather than `useResource`, so the header offers no Reload there, but it writes
  anyway: `ApplyYAML` reads `App.config.Path` per call, so a
  kubeconfig switch changes where the next manifest lands. Applying does not refresh the cached
  lists, because re-opening a menu reconnects its watch anyway.
- **The kubeconfig details live in the Settings page**, reachable from the sidebar footer and at
  the `/settings` route. The
  Settings page also lists, per resource kind, whether it has been loaded yet, how many items
  it holds and when it was refreshed. Resource pages show no connection chrome: a list that has
  not loaded yet shows the spinner, and a failed load shows the error with its own retry.
  Settings maps to no kube kind and has no backend method behind it, while the Manifest YAML page
  streams nothing and only calls `ApplyYAML`.
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
  a component overwrites it, so re-apply any local edits after `npx shadcn@latest add`. The one
  edit to remember is in `table.tsx`: the registry ships `p-2` cells (`h-10 px-2` heads), and the
  tables want roomier rows, so `TableCell` is `px-3 py-3` and `TableHead` is `h-12 px-3`. Every
  table in the app takes that padding from the component, so there is no per-page override to
  change.
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
  functions from `../wailsjs/go/main/App` (`../../wailsjs/go/main/App` from `src/pages`), types
  from the same depth of `../wailsjs/go/models`, events from `../wailsjs/runtime`. Pages take
  shared state from `@/app-context` rather than props through the router: a page that streams a
  kind calls `useResource("<kind>")` (`@/use-resource`), and a page that streams nothing, such
  as Settings, calls `useApp()`.
- Add new UI primitives with the registry instead of hand-writing markup:
  `cd frontend && npx shadcn@latest add <component>`. Treat the generated files under
  `src/components/ui` as owned source that can be edited, and keep them monochrome.

## Testing

`go test ./...` runs pure unit tests in `internal/kube`, no cluster required:

- `kubeconfig_test.go` covers the resolution precedence, including that a higher priority
  source wins, that a vanished manual pick falls back to discovery, and that lower priority
  files are not chosen.
- `apply_test.go` covers manifest decoding (a missing `apiVersion`/`kind`/`metadata.name`, JSON
  input, several documents, a trailing separator, a non-mapping document), the namespace
  defaulting and cluster-scoped stripping in `placement`, and kind resolution through a fake
  discovery client, including an unknown kind. The server-side apply itself is not exercised:
  the fake dynamic client does not reproduce its semantics.
- `pods_test.go` covers the kubectl-equivalent status computation (init containers, waiting
  and terminated reasons, completed pods that are still running, deletion states), the
  Ready/Restarts/Age/Node flattening, and snapshot sorting.
- `deployments_test.go` and `statefulsets_test.go` cover the workload flattening: the READY
  count when `spec.replicas` is absent, the UP-TO-DATE and AVAILABLE counters, Age, and the
  sorting of each snapshot.
- `nodes_test.go` covers the node STATUS computation (Ready, NotReady, Unknown, a cordoned
  node and other conditions being ignored), the ROLES column (role-label prefix, legacy label,
  sorting and `<none>`), the VERSION/Age flattening, and name sorting.
- `namespaces_test.go` covers the namespace flattening: the phase as the STATUS column, a
  namespace stuck in Active with a deletion timestamp, a namespace without a phase, Age, and
  name sorting. It also covers `DeleteNamespace` against the fake clientset: the namespace is
  gone afterwards, and a namespace that was not there yields an error naming it.
- `services_test.go` covers the service flattening the way kubectl prints it: the CLUSTER-IP
  column including `None` for a headless service, the EXTERNAL-IP column for every type
  (`<none>`, explicit external IPs, `<pending>` for a load balancer without an address, sorted
  and deduplicated ingress addresses, and the ExternalName target), the PORT(S) column with and
  without a node port, Age, and the namespace-then-name sorting.
- `metrics_test.go` covers the CPU and memory path. The sums and the formatting are checked
  against the same numbers kubectl's own printer test uses (0.2 + 0.2 cores becomes `400m`,
  1Gi + 1Gi becomes `2048Mi`), and the two `Usages` readers are tested against an `httptest`
  server, so the endpoint, the decode and the error wording are all exercised without a cluster:
  a 404 reports which column failed and a malformed body is an error too. There is nothing here
  about an interval, because the package no longer owns one.
- `gatewayapi_test.go` covers the helpers the four Gateway API kinds share: reading a condition by
  type (True, False, Unknown, the requested condition absent, and no conditions at all), the
  `<none>` placeholder for an empty address or hostname list, addresses and hostnames keeping
  their stored order, and the 404 mapping that turns a missing CRD into the not-installed message
  while letting every other error through untouched. It also covers `routeParentRefs`: a bare name
  resolving to the route's own namespace, an explicit namespace winning, a listener section being
  kept, and several references keeping their order.
- `gatewayclasses_test.go`, `gateways_test.go`, `httproutes_test.go` and `grpcroutes_test.go`
  cover each kind's flattening and store sorting: the gateway class CONTROLLER and ACCEPTED
  columns, the gateway CLASS/ADDRESS/PROGRAMMED columns including the placeholders for a gateway
  that has not been programmed yet, the hostnames and resolved parent references of both route
  kinds, and the cluster-scoped name sort against the namespace-then-name sort.
- `terminal_test.go` covers the terminal's own logic: the container defaulting (an empty request
  takes the first container, and a name the pod does not declare is refused by a message that
  names it), the shell that fills in for a missing command, and the size queue (the size the
  drawer already knows comes back from the first `Next` without waiting, a zero dimension is
  skipped rather than sent, and `Next` reports nil once the session ends so the stream's resize
  loop stops).
- `terminal_test.go` in `main` covers the transport with no cluster at all, because the cluster
  side is a `terminalRunner` field. A fake runner that echoes stdin proves the frame protocol end
  to end (a `ready` text frame first, binary input returning as binary output, a resize reaching
  the session, and `close` ending it as a clean exit), a runner that returns an `ExitError` proves
  the code survives to the client, and the ticket store is checked for single use, expiry and a
  404 for an unknown id. `allowedTerminalOrigin` is a table test.

There is no integration test infrastructure: `go test ./...` never talks to a cluster. A
throwaway test was used once to confirm that each watcher probes, syncs and publishes against
the real kubeconfig on this machine (pods, deployments, statefulsets, namespaces and services all
reached the cluster, and the metrics endpoints answered with real usage numbers), but it was
deleted rather than checked in.
