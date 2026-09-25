package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"antek-async/internal/kube"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// stateUpdateEvent carries every AppState change to the frontend.
const stateUpdateEvent = "state:update"

// The per-resource slices of the payload. Each kind gets its own concrete struct rather than one
// generic type because the binding generator cannot name a generic instantiation.
type PodsState struct {
	Items     []kube.PodInfo `json:"items"`
	Loaded    bool           `json:"loaded"`
	Loading   bool           `json:"loading"`
	Error     string         `json:"error"`
	UpdatedAt string         `json:"updatedAt"`
}

type DeploymentsState struct {
	Items     []kube.DeploymentInfo `json:"items"`
	Loaded    bool                  `json:"loaded"`
	Loading   bool                  `json:"loading"`
	Error     string                `json:"error"`
	UpdatedAt string                `json:"updatedAt"`
}

type StatefulSetsState struct {
	Items     []kube.StatefulSetInfo `json:"items"`
	Loaded    bool                   `json:"loaded"`
	Loading   bool                   `json:"loading"`
	Error     string                 `json:"error"`
	UpdatedAt string                 `json:"updatedAt"`
}

type NodesState struct {
	Items     []kube.NodeInfo `json:"items"`
	Loaded    bool            `json:"loaded"`
	Loading   bool            `json:"loading"`
	Error     string          `json:"error"`
	UpdatedAt string          `json:"updatedAt"`
}

type NamespacesState struct {
	Items     []kube.NamespaceInfo `json:"items"`
	Loaded    bool                 `json:"loaded"`
	Loading   bool                 `json:"loading"`
	Error     string               `json:"error"`
	UpdatedAt string               `json:"updatedAt"`
}

type ServicesState struct {
	Items     []kube.ServiceInfo `json:"items"`
	Loaded    bool               `json:"loaded"`
	Loading   bool               `json:"loading"`
	Error     string             `json:"error"`
	UpdatedAt string             `json:"updatedAt"`
}

type GatewayClassesState struct {
	Items     []kube.GatewayClassInfo `json:"items"`
	Loaded    bool                    `json:"loaded"`
	Loading   bool                    `json:"loading"`
	Error     string                  `json:"error"`
	UpdatedAt string                  `json:"updatedAt"`
}

type GatewaysState struct {
	Items     []kube.GatewayInfo `json:"items"`
	Loaded    bool               `json:"loaded"`
	Loading   bool               `json:"loading"`
	Error     string             `json:"error"`
	UpdatedAt string             `json:"updatedAt"`
}

type HTTPRoutesState struct {
	Items     []kube.HTTPRouteInfo `json:"items"`
	Loaded    bool                 `json:"loaded"`
	Loading   bool                 `json:"loading"`
	Error     string               `json:"error"`
	UpdatedAt string               `json:"updatedAt"`
}

type GRPCRoutesState struct {
	Items     []kube.GRPCRouteInfo `json:"items"`
	Loaded    bool                 `json:"loaded"`
	Loading   bool                 `json:"loading"`
	Error     string               `json:"error"`
	UpdatedAt string               `json:"updatedAt"`
}

// AppState is the single payload shared by GetState and the state:update event, so the
// frontend needs one reducer and never has to merge racing updates.
type AppState struct {
	Config         kube.Status         `json:"config"`
	Active         string              `json:"active"`
	Pods           PodsState           `json:"pods"`
	Deployments    DeploymentsState    `json:"deployments"`
	StatefulSets   StatefulSetsState   `json:"statefulSets"`
	Nodes          NodesState          `json:"nodes"`
	Namespaces     NamespacesState     `json:"namespaces"`
	Services       ServicesState       `json:"services"`
	GatewayClasses GatewayClassesState `json:"gatewayClasses"`
	Gateways       GatewaysState       `json:"gateways"`
	HTTPRoutes     HTTPRoutesState     `json:"httpRoutes"`
	GRPCRoutes     GRPCRoutesState     `json:"grpcRoutes"`
	Error          string              `json:"error"`
}

// resourceHolder is the cached list plus the watch bookkeeping for one resource kind. Only one
// resource is watched at a time, and every field is guarded by App.mu.
type resourceHolder[T any] struct {
	items     []T
	loaded    bool
	loading   bool
	err       string
	updatedAt string
	cancel    context.CancelFunc
}

// App struct
type App struct {
	ctx context.Context

	mu         sync.RWMutex
	manualPath string // kubeconfig picked in the dialog, overrides auto-discovery
	config     kube.Status
	active     kube.Resource
	lastError  string

	// generation identifies the active watch, so a superseded one cannot publish.
	generation int

	pods           resourceHolder[kube.PodInfo]
	deployments    resourceHolder[kube.DeploymentInfo]
	statefulSets   resourceHolder[kube.StatefulSetInfo]
	nodes          resourceHolder[kube.NodeInfo]
	namespaces     resourceHolder[kube.NamespaceInfo]
	services       resourceHolder[kube.ServiceInfo]
	gatewayClasses resourceHolder[kube.GatewayClassInfo]
	gateways       resourceHolder[kube.GatewayInfo]
	httpRoutes     resourceHolder[kube.HTTPRouteInfo]
	grpcRoutes     resourceHolder[kube.GRPCRouteInfo]

	// terminals serves the interactive sessions. It is outside the state above because a
	// terminal is not a list: it is a live connection the drawer owns until it closes.
	terminals *terminalServer
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{terminals: newTerminalServer()}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Pods are the view the frontend opens first, so starting that watch here means the list is
	// usually ready by the time the webview paints. The pods page then asks for the same kind
	// like every other page, which reconnects this watcher onto a fresh list.
	a.mu.Lock()
	a.config = kube.Resolve(a.manualPath)
	a.active = kube.ResourcePods
	a.mu.Unlock()

	a.startWatch(kube.ResourcePods)
}

// shutdown stops the running watcher and every open terminal. The startup context is never
// cancelled by Wails, so both have to be stopped here.
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	a.cancelWatchesLocked()
	a.mu.Unlock()

	a.terminals.close()
}

// GetState returns the config and every cached resource list as one snapshot. The frontend
// calls it on mount because events sent before it subscribed are lost.
func (a *App) GetState() AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.stateLocked()
}

// SelectResource switches which resource is fetched: the previous watcher is cancelled and the
// requested one starts. Selecting the active resource again reconnects it, which is what the
// retry button needs.
func (a *App) SelectResource(resource string) AppState {
	requested := kube.Resource(resource)
	if !requested.Valid() {
		return a.GetState()
	}

	a.mu.Lock()
	a.active = requested
	a.mu.Unlock()

	a.startWatch(requested)

	return a.GetState()
}

// PickKubeconfig lets the user choose a kubeconfig file and switches to it for the rest of the
// session. Cancelling the dialog leaves the state untouched, because Wails reports a cancelled
// dialog as an empty path with a nil error.
func (a *App) PickKubeconfig() AppState {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choose kubeconfig file",
		DefaultDirectory: kube.DefaultConfigDir(),
		Filters: []runtime.FileFilter{
			{DisplayName: "Kubeconfig (*.yaml, *.yml, *.conf)", Pattern: "*.yaml;*.yml;*.conf"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
	if err != nil {
		a.setError(err)
		return a.GetState()
	}
	if path == "" {
		return a.GetState()
	}

	a.mu.Lock()
	a.manualPath = path
	a.config = kube.Resolve(a.manualPath)
	a.resetResourcesLocked()
	active := a.active
	a.mu.Unlock()

	// A shell belongs to the cluster it was started on, so the switch ends every session. It is
	// done outside the lock because closing a session reaches into the terminal server.
	a.terminals.closeSessions()

	a.startWatch(active)

	return a.GetState()
}

// ResetKubeconfig drops the manual selection and falls back to auto-discovery.
func (a *App) ResetKubeconfig() AppState {
	a.mu.Lock()
	a.manualPath = ""
	a.config = kube.Resolve(a.manualPath)
	a.resetResourcesLocked()
	active := a.active
	a.mu.Unlock()

	// The sessions opened against the cluster that was just dropped have to go with it.
	a.terminals.closeSessions()

	a.startWatch(active)

	return a.GetState()
}

// GetPodYAML returns one pod as YAML for the read-only viewer. It builds a short-lived client
// instead of reusing the watch, because the viewer can be opened for a pod that the active menu
// (for example Nodes) is not streaming.
func (a *App) GetPodYAML(namespace, name string) (string, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return "", errors.New("Kubeconfig not found")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return "", err
	}

	return kube.PodYAML(a.ctx, clientset, namespace, name)
}

// gatewayClient builds the Gateway API client for one on-demand request. Like GetPodYAML it reads
// the kubeconfig per call, so the request lands on the cluster the user is looking at rather than
// on whichever watch happens to be running.
func (a *App) gatewayClient() (gatewayclient.Interface, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return nil, errors.New("Kubeconfig not found")
	}

	return kube.GatewayClientFor(path)
}

// GetGatewayClassYAML returns one gateway class as YAML for the read-only viewer. GatewayClass is
// cluster scoped, so it takes no namespace.
func (a *App) GetGatewayClassYAML(name string) (string, error) {
	clientset, err := a.gatewayClient()
	if err != nil {
		return "", err
	}

	return kube.GatewayClassYAML(a.ctx, clientset, name)
}

// GetGatewayYAML returns one gateway as YAML for the read-only viewer.
func (a *App) GetGatewayYAML(namespace, name string) (string, error) {
	clientset, err := a.gatewayClient()
	if err != nil {
		return "", err
	}

	return kube.GatewayYAML(a.ctx, clientset, namespace, name)
}

// GetHTTPRouteYAML returns one HTTP route as YAML for the read-only viewer.
func (a *App) GetHTTPRouteYAML(namespace, name string) (string, error) {
	clientset, err := a.gatewayClient()
	if err != nil {
		return "", err
	}

	return kube.HTTPRouteYAML(a.ctx, clientset, namespace, name)
}

// GetGRPCRouteYAML returns one gRPC route as YAML for the read-only viewer.
func (a *App) GetGRPCRouteYAML(namespace, name string) (string, error) {
	clientset, err := a.gatewayClient()
	if err != nil {
		return "", err
	}

	return kube.GRPCRouteYAML(a.ctx, clientset, namespace, name)
}

// GetPodContainers lists the containers a terminal can target in one pod, so a pod with sidecars
// still gets a picker. Like GetPodYAML it builds a short-lived client, because it is not tied to
// whichever watch happens to be running.
func (a *App) GetPodContainers(namespace, name string) ([]string, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return nil, errors.New("Kubeconfig not found")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return nil, err
	}

	return kube.PodContainers(a.ctx, clientset, namespace, name)
}

// OpenTerminal validates a terminal request and returns the one-time endpoint that starts it.
// Nothing is opened against the cluster until the drawer connects to that endpoint, so a drawer
// closed while it is still starting leaves no session behind. The kubeconfig is read per call, so
// the session is pinned to the cluster the user was looking at, not to a later switch.
func (a *App) OpenTerminal(request kube.TerminalRequest) (TerminalEndpoint, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	return a.terminals.prepare(a.ctx, path, request)
}

// ApplyYAML sends one manifest to the active cluster with server-side apply, so the editor can
// create or update any kind the cluster knows. It reads the current kubeconfig per call, so
// switching clusters changes where the document lands. The error is returned to the caller
// rather than stored in AppState, because it belongs to the editor page and not to the shell.
func (a *App) ApplyYAML(document string) (kube.ApplyResult, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return kube.ApplyResult{}, errors.New("Kubeconfig not found")
	}

	// force stays off: a field owned by another manager should surface as a conflict the user
	// can read, not be stolen silently.
	return kube.ApplyYAML(a.ctx, path, document, false)
}

// GetPodUsages returns the CPU and memory metrics-server reports for every pod, already
// formatted the way kubectl prints them. Metrics have no watch, so this is a request the pods
// page makes on its own schedule, which is what lets it stop asking while the window is not
// being looked at. Like GetPodYAML it builds a short-lived client, because the call is not tied
// to which watch happens to be running.
func (a *App) GetPodUsages() ([]kube.PodUsage, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return nil, errors.New("Kubeconfig not found")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return nil, err
	}

	return kube.PodUsages(a.ctx, clientset)
}

// GetNodeUsages is GetPodUsages for the node metrics.
func (a *App) GetNodeUsages() ([]kube.NodeUsage, error) {
	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return nil, errors.New("Kubeconfig not found")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return nil, err
	}

	return kube.NodeUsages(a.ctx, clientset)
}

// DeleteNamespace removes one namespace from the cluster the active kubeconfig points at. Like
// GetPodYAML it builds a short-lived client, because the namespace page's watch may not be the
// one that is running. The error goes back to the caller, so the confirmation dialog can report
// it instead of the shared error banner.
func (a *App) DeleteNamespace(name string) error {
	if name == "" {
		return errors.New("Namespace name is required")
	}

	a.mu.RLock()
	path := a.config.Path
	a.mu.RUnlock()

	if path == "" {
		return errors.New("Kubeconfig not found")
	}

	clientset, err := kube.ClientFor(path)
	if err != nil {
		return err
	}

	return kube.DeleteNamespace(a.ctx, clientset, name)
}

// startWatch cancels every other watcher and streams the requested resource, so only the menu
// that is open ever talks to the cluster.
func (a *App) startWatch(resource kube.Resource) {
	a.mu.Lock()
	a.cancelWatchesLocked()

	path := a.config.Path
	a.generation++
	generation := a.generation
	a.mu.Unlock()

	// Without a kubeconfig there is nothing to stream, so report it against the requested
	// resource instead of leaving a loading state behind.
	if path == "" {
		a.failResource(resource, generation, errors.New("Kubeconfig not found"))
		return
	}

	a.mu.Lock()
	ctx, cancel := context.WithCancel(a.ctx)

	// The holder is picked by kind, which is also the only place the list type is known.
	switch resource {
	case kube.ResourcePods:
		a.pods.cancel, a.pods.loading, a.pods.err = cancel, true, ""
	case kube.ResourceDeployments:
		a.deployments.cancel, a.deployments.loading, a.deployments.err = cancel, true, ""
	case kube.ResourceStatefulSets:
		a.statefulSets.cancel, a.statefulSets.loading, a.statefulSets.err = cancel, true, ""
	case kube.ResourceNodes:
		a.nodes.cancel, a.nodes.loading, a.nodes.err = cancel, true, ""
	case kube.ResourceNamespaces:
		a.namespaces.cancel, a.namespaces.loading, a.namespaces.err = cancel, true, ""
	case kube.ResourceServices:
		a.services.cancel, a.services.loading, a.services.err = cancel, true, ""
	case kube.ResourceGatewayClasses:
		a.gatewayClasses.cancel, a.gatewayClasses.loading, a.gatewayClasses.err = cancel, true, ""
	case kube.ResourceGateways:
		a.gateways.cancel, a.gateways.loading, a.gateways.err = cancel, true, ""
	case kube.ResourceHTTPRoutes:
		a.httpRoutes.cancel, a.httpRoutes.loading, a.httpRoutes.err = cancel, true, ""
	case kube.ResourceGRPCRoutes:
		a.grpcRoutes.cancel, a.grpcRoutes.loading, a.grpcRoutes.err = cancel, true, ""
	default:
		// Not a resource this app watches, so drop the context that was just built.
		cancel()
		a.mu.Unlock()
		return
	}

	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)

	go a.streamResource(ctx, resource, path, generation)
}

// streamResource builds one client and watches a single resource until ctx is done.
func (a *App) streamResource(ctx context.Context, resource kube.Resource, path string, generation int) {
	clientset, err := kube.ClientFor(path)
	if err != nil {
		a.failResource(resource, generation, err)
		return
	}

	var watchErr error
	switch resource {
	case kube.ResourcePods:
		watchErr = kube.WatchPods(ctx, clientset, func(items []kube.PodInfo) {
			a.publishPods(generation, items)
		})
	case kube.ResourceDeployments:
		watchErr = kube.WatchDeployments(ctx, clientset, func(items []kube.DeploymentInfo) {
			a.publishDeployments(generation, items)
		})
	case kube.ResourceStatefulSets:
		watchErr = kube.WatchStatefulSets(ctx, clientset, func(items []kube.StatefulSetInfo) {
			a.publishStatefulSets(generation, items)
		})
	case kube.ResourceNodes:
		watchErr = kube.WatchNodes(ctx, clientset, func(items []kube.NodeInfo) {
			a.publishNodes(generation, items)
		})
	case kube.ResourceNamespaces:
		watchErr = kube.WatchNamespaces(ctx, clientset, func(items []kube.NamespaceInfo) {
			a.publishNamespaces(generation, items)
		})
	case kube.ResourceServices:
		watchErr = kube.WatchServices(ctx, clientset, func(items []kube.ServiceInfo) {
			a.publishServices(generation, items)
		})
	case kube.ResourceGatewayClasses, kube.ResourceGateways, kube.ResourceHTTPRoutes, kube.ResourceGRPCRoutes:
		// The Gateway API kinds need their own clientset, so all four are streamed from one helper
		// instead of repeating the client construction four times. The core clientset built above
		// goes unused for them, which costs one kubeconfig read per menu open.
		watchErr = a.watchGatewayAPI(ctx, resource, path, generation)
	}

	if watchErr != nil && ctx.Err() == nil {
		a.failResource(resource, generation, watchErr)
	}
}

// watchGatewayAPI builds the Gateway API client and streams the requested kind with it.
func (a *App) watchGatewayAPI(ctx context.Context, resource kube.Resource, path string, generation int) error {
	clientset, err := kube.GatewayClientFor(path)
	if err != nil {
		return err
	}

	switch resource {
	case kube.ResourceGatewayClasses:
		return kube.WatchGatewayClasses(ctx, clientset, func(items []kube.GatewayClassInfo) {
			a.publishGatewayClasses(generation, items)
		})
	case kube.ResourceGateways:
		return kube.WatchGateways(ctx, clientset, func(items []kube.GatewayInfo) {
			a.publishGateways(generation, items)
		})
	case kube.ResourceHTTPRoutes:
		return kube.WatchHTTPRoutes(ctx, clientset, func(items []kube.HTTPRouteInfo) {
			a.publishHTTPRoutes(generation, items)
		})
	case kube.ResourceGRPCRoutes:
		return kube.WatchGRPCRoutes(ctx, clientset, func(items []kube.GRPCRouteInfo) {
			a.publishGRPCRoutes(generation, items)
		})
	}

	return nil
}

func (a *App) publishPods(generation int, items []kube.PodInfo) {
	publishResource(a, &a.pods, generation, items)
}

func (a *App) publishDeployments(generation int, items []kube.DeploymentInfo) {
	publishResource(a, &a.deployments, generation, items)
}

func (a *App) publishStatefulSets(generation int, items []kube.StatefulSetInfo) {
	publishResource(a, &a.statefulSets, generation, items)
}

func (a *App) publishNodes(generation int, items []kube.NodeInfo) {
	publishResource(a, &a.nodes, generation, items)
}

func (a *App) publishNamespaces(generation int, items []kube.NamespaceInfo) {
	publishResource(a, &a.namespaces, generation, items)
}

func (a *App) publishServices(generation int, items []kube.ServiceInfo) {
	publishResource(a, &a.services, generation, items)
}

func (a *App) publishGatewayClasses(generation int, items []kube.GatewayClassInfo) {
	publishResource(a, &a.gatewayClasses, generation, items)
}

func (a *App) publishGateways(generation int, items []kube.GatewayInfo) {
	publishResource(a, &a.gateways, generation, items)
}

func (a *App) publishHTTPRoutes(generation int, items []kube.HTTPRouteInfo) {
	publishResource(a, &a.httpRoutes, generation, items)
}

func (a *App) publishGRPCRoutes(generation int, items []kube.GRPCRouteInfo) {
	publishResource(a, &a.grpcRoutes, generation, items)
}

// failResource records why a resource could not be streamed.
func (a *App) failResource(resource kube.Resource, generation int, err error) {
	switch resource {
	case kube.ResourcePods:
		failResource(a, &a.pods, generation, err)
	case kube.ResourceDeployments:
		failResource(a, &a.deployments, generation, err)
	case kube.ResourceStatefulSets:
		failResource(a, &a.statefulSets, generation, err)
	case kube.ResourceNodes:
		failResource(a, &a.nodes, generation, err)
	case kube.ResourceNamespaces:
		failResource(a, &a.namespaces, generation, err)
	case kube.ResourceServices:
		failResource(a, &a.services, generation, err)
	case kube.ResourceGatewayClasses:
		failResource(a, &a.gatewayClasses, generation, err)
	case kube.ResourceGateways:
		failResource(a, &a.gateways, generation, err)
	case kube.ResourceHTTPRoutes:
		failResource(a, &a.httpRoutes, generation, err)
	case kube.ResourceGRPCRoutes:
		failResource(a, &a.grpcRoutes, generation, err)
	}
}

// cancelWatchesLocked stops whichever watcher is running. Callers must hold a.mu.
func (a *App) cancelWatchesLocked() {
	for _, cancel := range []context.CancelFunc{a.pods.cancel, a.deployments.cancel, a.statefulSets.cancel, a.nodes.cancel, a.namespaces.cancel, a.services.cancel, a.gatewayClasses.cancel, a.gateways.cancel, a.httpRoutes.cancel, a.grpcRoutes.cancel} {
		if cancel != nil {
			cancel()
		}
	}

	a.pods.cancel = nil
	a.deployments.cancel = nil
	a.statefulSets.cancel = nil
	a.nodes.cancel = nil
	a.namespaces.cancel = nil
	a.services.cancel = nil
	a.gatewayClasses.cancel = nil
	a.gateways.cancel = nil
	a.httpRoutes.cancel = nil
	a.grpcRoutes.cancel = nil
}

// resetResourcesLocked drops every cached list, because they belong to the previous cluster.
// Callers must hold a.mu.
func (a *App) resetResourcesLocked() {
	a.cancelWatchesLocked()

	a.pods = resourceHolder[kube.PodInfo]{}
	a.deployments = resourceHolder[kube.DeploymentInfo]{}
	a.statefulSets = resourceHolder[kube.StatefulSetInfo]{}
	a.nodes = resourceHolder[kube.NodeInfo]{}
	a.namespaces = resourceHolder[kube.NamespaceInfo]{}
	a.services = resourceHolder[kube.ServiceInfo]{}
	a.gatewayClasses = resourceHolder[kube.GatewayClassInfo]{}
	a.gateways = resourceHolder[kube.GatewayInfo]{}
	a.httpRoutes = resourceHolder[kube.HTTPRouteInfo]{}
	a.grpcRoutes = resourceHolder[kube.GRPCRouteInfo]{}
}

// setError records a recoverable failure, such as a dialog that could not be opened.
func (a *App) setError(err error) {
	a.mu.Lock()
	a.lastError = err.Error()
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// emit sends the state to the frontend. The Wails runtime only accepts the context handed to
// the lifecycle hooks, so a.ctx is used even from watcher goroutines.
func (a *App) emit(state AppState) {
	runtime.EventsEmit(a.ctx, stateUpdateEvent, state)
}

// stateLocked snapshots the state. Callers must hold a.mu.
func (a *App) stateLocked() AppState {
	return AppState{
		Config: a.config,
		Active: string(a.active),
		Pods: PodsState{
			Items:     nonNil(a.pods.items),
			Loaded:    a.pods.loaded,
			Loading:   a.pods.loading,
			Error:     a.pods.err,
			UpdatedAt: a.pods.updatedAt,
		},
		Deployments: DeploymentsState{
			Items:     nonNil(a.deployments.items),
			Loaded:    a.deployments.loaded,
			Loading:   a.deployments.loading,
			Error:     a.deployments.err,
			UpdatedAt: a.deployments.updatedAt,
		},
		StatefulSets: StatefulSetsState{
			Items:     nonNil(a.statefulSets.items),
			Loaded:    a.statefulSets.loaded,
			Loading:   a.statefulSets.loading,
			Error:     a.statefulSets.err,
			UpdatedAt: a.statefulSets.updatedAt,
		},
		Nodes: NodesState{
			Items:     nonNil(a.nodes.items),
			Loaded:    a.nodes.loaded,
			Loading:   a.nodes.loading,
			Error:     a.nodes.err,
			UpdatedAt: a.nodes.updatedAt,
		},
		Namespaces: NamespacesState{
			Items:     nonNil(a.namespaces.items),
			Loaded:    a.namespaces.loaded,
			Loading:   a.namespaces.loading,
			Error:     a.namespaces.err,
			UpdatedAt: a.namespaces.updatedAt,
		},
		Services: ServicesState{
			Items:     nonNil(a.services.items),
			Loaded:    a.services.loaded,
			Loading:   a.services.loading,
			Error:     a.services.err,
			UpdatedAt: a.services.updatedAt,
		},
		GatewayClasses: GatewayClassesState{
			Items:     nonNil(a.gatewayClasses.items),
			Loaded:    a.gatewayClasses.loaded,
			Loading:   a.gatewayClasses.loading,
			Error:     a.gatewayClasses.err,
			UpdatedAt: a.gatewayClasses.updatedAt,
		},
		Gateways: GatewaysState{
			Items:     nonNil(a.gateways.items),
			Loaded:    a.gateways.loaded,
			Loading:   a.gateways.loading,
			Error:     a.gateways.err,
			UpdatedAt: a.gateways.updatedAt,
		},
		HTTPRoutes: HTTPRoutesState{
			Items:     nonNil(a.httpRoutes.items),
			Loaded:    a.httpRoutes.loaded,
			Loading:   a.httpRoutes.loading,
			Error:     a.httpRoutes.err,
			UpdatedAt: a.httpRoutes.updatedAt,
		},
		GRPCRoutes: GRPCRoutesState{
			Items:     nonNil(a.grpcRoutes.items),
			Loaded:    a.grpcRoutes.loaded,
			Loading:   a.grpcRoutes.loading,
			Error:     a.grpcRoutes.err,
			UpdatedAt: a.grpcRoutes.updatedAt,
		},
		Error: a.lastError,
	}
}

// publishResource stores a fresh list for one resource and pushes the state to the frontend.
// It is a free function because Go methods cannot take type parameters, and it drops results
// from a superseded watch so a slow watcher cannot overwrite newer data.
func publishResource[T any](a *App, holder *resourceHolder[T], generation int, items []T) {
	a.mu.Lock()
	if generation != a.generation {
		a.mu.Unlock()
		return
	}

	holder.items = items
	holder.loaded = true
	holder.loading = false
	holder.err = ""
	holder.updatedAt = time.Now().Format(time.RFC3339)
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// failResource marks a resource as failed to load. The previous list is kept so the table stays
// on screen next to the error.
func failResource[T any](a *App, holder *resourceHolder[T], generation int, err error) {
	a.mu.Lock()
	if generation != a.generation {
		a.mu.Unlock()
		return
	}

	holder.loading = false
	holder.err = err.Error()
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// nonNil keeps the JSON payload free of nulls so the frontend can map over every list.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}

	return items
}
