package main

import (
	"context"
	"sync"
	"time"

	"antek-async/internal/kube"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// stateUpdateEvent carries every AppState change to the frontend.
const stateUpdateEvent = "state:update"

// AppState is the single payload shared by GetState and the state:update event, so
// the frontend needs one reducer and never has to merge racing updates.
type AppState struct {
	Config    kube.Status    `json:"config"`
	Pods      []kube.PodInfo `json:"pods"`
	Connected bool           `json:"connected"`
	UpdatedAt string         `json:"updatedAt"`
	Error     string         `json:"error"`
}

// App struct
type App struct {
	ctx context.Context

	mu         sync.RWMutex
	manualPath string // kubeconfig picked in the dialog, overrides auto-discovery
	config     kube.Status
	pods       []kube.PodInfo // last successful list, kept on screen during errors
	connected  bool
	updatedAt  string
	lastError  string

	// generation identifies the active watch, so a superseded one cannot publish.
	generation int

	cancelWatch context.CancelFunc
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.mu.Lock()
	a.config = kube.Resolve(a.manualPath)
	a.mu.Unlock()

	a.restartWatch()
}

// shutdown stops the pod watch. The startup context is never cancelled by Wails,
// so the watcher has to be stopped here.
func (a *App) shutdown(context.Context) {
	a.stopWatch()
}

// GetState returns the config, pod list and connection status as one snapshot.
// The frontend calls it on mount because events sent before it subscribed are lost.
func (a *App) GetState() AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.stateLocked()
}

// PickKubeconfig lets the user choose a kubeconfig file and switches to it for the
// rest of the session. Cancelling the dialog leaves the state untouched, because
// Wails reports a cancelled dialog as an empty path with a nil error.
func (a *App) PickKubeconfig() AppState {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Pilih file kubeconfig",
		DefaultDirectory: kube.DefaultConfigDir(),
		Filters: []runtime.FileFilter{
			{DisplayName: "Kubeconfig (*.yaml, *.yml, *.conf)", Pattern: "*.yaml;*.yml;*.conf"},
			{DisplayName: "Semua file", Pattern: "*.*"},
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
	a.mu.Unlock()

	a.restartWatch()

	return a.GetState()
}

// ResetKubeconfig drops the manual selection and falls back to auto-discovery.
func (a *App) ResetKubeconfig() AppState {
	a.mu.Lock()
	a.manualPath = ""
	a.config = kube.Resolve(a.manualPath)
	a.mu.Unlock()

	a.restartWatch()

	return a.GetState()
}

// RefreshPods reconnects by restarting the watch, which is the retry action the UI
// offers after a connection error.
func (a *App) RefreshPods() AppState {
	a.restartWatch()

	return a.GetState()
}

// restartWatch replaces the running watcher with one bound to the current kubeconfig.
func (a *App) restartWatch() {
	a.stopWatch()

	a.mu.Lock()
	path := a.config.Path
	a.generation++
	generation := a.generation
	a.connected = false
	if path == "" {
		a.lastError = a.config.Error
		if a.lastError == "" {
			a.lastError = "Kubeconfig tidak ditemukan"
		}
	} else {
		a.lastError = ""
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelWatch = cancel
	a.mu.Unlock()

	a.emit(a.GetState())

	if path == "" {
		return
	}

	go func() {
		err := kube.Watch(ctx, path, func(pods []kube.PodInfo) {
			a.publish(generation, pods)
		})
		if err != nil && ctx.Err() == nil {
			a.setDisconnected(generation, err)
		}
	}()
}

// stopWatch cancels the running watcher, if any.
func (a *App) stopWatch() {
	a.mu.Lock()
	cancel := a.cancelWatch
	a.cancelWatch = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// publish stores a fresh snapshot and pushes it to the frontend. Snapshots from a
// superseded watch are dropped, so a slow watcher cannot overwrite newer data.
func (a *App) publish(generation int, pods []kube.PodInfo) {
	a.mu.Lock()
	if generation != a.generation {
		a.mu.Unlock()
		return
	}

	a.pods = pods
	a.connected = true
	a.lastError = ""
	a.updatedAt = time.Now().Format(time.RFC3339)
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// setDisconnected records a connection failure. The last known pods are kept so the
// table stays on screen next to the error banner. Failures from a superseded watch
// are ignored for the same reason as in publish.
func (a *App) setDisconnected(generation int, err error) {
	a.mu.Lock()
	if generation != a.generation {
		a.mu.Unlock()
		return
	}

	a.connected = false
	a.lastError = err.Error()
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// setError records a recoverable failure, such as a dialog that could not be opened.
func (a *App) setError(err error) {
	a.mu.Lock()
	a.lastError = err.Error()
	state := a.stateLocked()
	a.mu.Unlock()

	a.emit(state)
}

// emit sends the state to the frontend. The Wails runtime only accepts the context
// handed to the lifecycle hooks, so a.ctx is used even from watcher goroutines.
func (a *App) emit(state AppState) {
	runtime.EventsEmit(a.ctx, stateUpdateEvent, state)
}

// stateLocked snapshots the state. Callers must hold a.mu.
func (a *App) stateLocked() AppState {
	pods := a.pods
	if pods == nil {
		pods = []kube.PodInfo{}
	}

	return AppState{
		Config:    a.config,
		Pods:      pods,
		Connected: a.connected,
		UpdatedAt: a.updatedAt,
		Error:     a.lastError,
	}
}
