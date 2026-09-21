package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"antek-async/internal/kube"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// stateUpdateEvent carries every AppState change to the frontend.
const stateUpdateEvent = "state:update"

// PodsState, DeploymentsState and StatefulSetsState are the per-resource slices of the payload.
// They are concrete structs rather than one generic type because the binding generator cannot
// name a generic instantiation.
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

// AppState is the single payload shared by GetState and the state:update event, so the
// frontend needs one reducer and never has to merge racing updates.
type AppState struct {
	Config       kube.Status       `json:"config"`
	Active       string            `json:"active"`
	Pods         PodsState         `json:"pods"`
	Deployments  DeploymentsState  `json:"deployments"`
	StatefulSets StatefulSetsState `json:"statefulSets"`
	Nodes        NodesState        `json:"nodes"`
	Namespaces   NamespacesState   `json:"namespaces"`
	Error        string            `json:"error"`
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

	pods         resourceHolder[kube.PodInfo]
	deployments  resourceHolder[kube.DeploymentInfo]
	statefulSets resourceHolder[kube.StatefulSetInfo]
	nodes        resourceHolder[kube.NodeInfo]
	namespaces   resourceHolder[kube.NamespaceInfo]
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
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

// shutdown stops the running watcher. The startup context is never cancelled by Wails, so the
// watcher has to be stopped here.
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	a.cancelWatchesLocked()
	a.mu.Unlock()
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
	}

	if watchErr != nil && ctx.Err() == nil {
		a.failResource(resource, generation, watchErr)
	}
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
	}
}

// cancelWatchesLocked stops whichever watcher is running. Callers must hold a.mu.
func (a *App) cancelWatchesLocked() {
	for _, cancel := range []context.CancelFunc{a.pods.cancel, a.deployments.cancel, a.statefulSets.cancel, a.nodes.cancel, a.namespaces.cancel} {
		if cancel != nil {
			cancel()
		}
	}

	a.pods.cancel = nil
	a.deployments.cancel = nil
	a.statefulSets.cancel = nil
	a.nodes.cancel = nil
	a.namespaces.cancel = nil
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
