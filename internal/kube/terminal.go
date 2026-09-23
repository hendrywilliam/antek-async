package kube

import (
	"context"
	"errors"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// defaultTerminalCommand is what a terminal runs when the caller names no command. /bin/sh is
// used rather than bash because it is the one shell every image that is not distroless carries,
// so a terminal works on more pods without asking the user to type a command.
var defaultTerminalCommand = []string{"/bin/sh"}

// TerminalRequest names one interactive session. Cols and Rows are the terminal's dimensions
// before it starts: the API server allocates the PTY up front, so the first size cannot wait for
// a resize message the way later ones do.
type TerminalRequest struct {
	Namespace string   `json:"namespace"`
	Pod       string   `json:"pod"`
	Container string   `json:"container"`
	Command   []string `json:"command"`
	Cols      uint16   `json:"cols"`
	Rows      uint16   `json:"rows"`
}

// TermSize is one change to the terminal's dimensions, in the columns and rows a terminal counts
// in.
type TermSize struct {
	Cols uint16
	Rows uint16
}

// TermStreams is the plumbing one session runs on. The cluster layer only reads stdin, writes
// output and drains the sizes, so it never learns how those bytes reach the window. That is what
// keeps this package free of the transport.
type TermStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Sizes  <-chan TermSize
}

// PodContainers lists the containers of one pod that a terminal can target. Init containers are
// left out because they run to completion before the pod is up, so a shell in one is never
// something the user is reaching for.
func PodContainers(ctx context.Context, clientset kubernetes.Interface, namespace, name string) ([]string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	pod, err := clientset.CoreV1().Pods(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("cannot read Pod %s/%s: %w", namespace, name, err)
	}

	containers := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
	}

	return containers, nil
}

// ResolveContainer checks a requested container against the pod and falls back to the first one,
// so the picker does not have to send a choice before the user has made one. A name the pod does
// not declare is refused here, where the message can say which container is missing, instead of
// by an exec request that would only fail on the wire.
func ResolveContainer(containers []string, wanted string) (string, error) {
	if len(containers) == 0 {
		return "", errors.New("Pod has no containers")
	}

	if wanted == "" {
		return containers[0], nil
	}

	for _, container := range containers {
		if container == wanted {
			return wanted, nil
		}
	}

	return "", fmt.Errorf("Container %s is not in this pod", wanted)
}

// ExecTerminal runs a shell (or the requested command) in one container with a TTY and streams
// until ctx is done or the process exits. It takes the kubeconfig path rather than a client
// because the executor needs the rest.Config as well as the REST client that builds the exec
// URL, which is the same reason ApplyYAML takes a path.
func ExecTerminal(ctx context.Context, path string, request TerminalRequest, streams TermStreams) error {
	restConfig, err := RestConfigFor(path)
	if err != nil {
		return err
	}

	// restConfig.Timeout stays unset the way RestConfigFor leaves it: it also applies to this
	// stream and would end a session on every interval. The session is bounded by ctx, which
	// lives exactly as long as the drawer that opened it.
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	executor, err := terminalExecutor(restConfig, clientset, request)
	if err != nil {
		return err
	}

	// stderr points at stdout because a TTY has one output stream: whatever the process writes
	// arrives in the order it wrote it.
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             streams.Stdin,
		Stdout:            streams.Stdout,
		Stderr:            streams.Stdout,
		Tty:               true,
		TerminalSizeQueue: newSizeQueue(ctx, request.Cols, request.Rows, streams.Sizes),
	})
}

// terminalExecutor builds the executor kubectl builds for exec: a WebSocket upgrade with the
// legacy SPDY protocol behind it, so a cluster or a proxy that cannot upgrade still gets a
// terminal. Only an upgrade failure falls back, because an error from inside the container has
// to reach the drawer instead of being retried on the other protocol.
func terminalExecutor(restConfig *rest.Config, clientset kubernetes.Interface, request TerminalRequest) (remotecommand.Executor, error) {
	execRequest := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(request.Namespace).
		Name(request.Pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: request.Container,
			Command:   request.command(),
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	spdy, err := remotecommand.NewSPDYExecutor(restConfig, "POST", execRequest.URL())
	if err != nil {
		return nil, err
	}

	// RFC 6455 requires the upgrade that carries the WebSocket protocol to be a GET.
	websocket, err := remotecommand.NewWebSocketExecutor(restConfig, "GET", execRequest.URL().String())
	if err != nil {
		return nil, err
	}

	return remotecommand.NewFallbackExecutor(websocket, spdy, func(err error) bool {
		return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
	})
}

// command is what the session runs, with the shell filled in when the caller sent none.
func (r TerminalRequest) command() []string {
	if len(r.Command) == 0 {
		return defaultTerminalCommand
	}

	return r.Command
}

// sizeQueue feeds the API server the terminal's dimensions. The size the drawer already knows
// goes in first, because the PTY is allocated before any output can arrive; changes after that
// are drained as they come. Next reports nil once the session is over, which is how the stream's
// resize loop learns to stop.
type sizeQueue struct {
	ctx   context.Context
	sizes <-chan TermSize
	first *remotecommand.TerminalSize
}

func newSizeQueue(ctx context.Context, cols, rows uint16, sizes <-chan TermSize) *sizeQueue {
	queue := &sizeQueue{ctx: ctx, sizes: sizes}

	// A missing size is left out rather than sent as zero, which would collapse the PTY.
	if cols > 0 && rows > 0 {
		queue.first = &remotecommand.TerminalSize{Width: cols, Height: rows}
	}

	return queue
}

func (q *sizeQueue) Next() *remotecommand.TerminalSize {
	if q.first != nil {
		first := q.first
		q.first = nil
		return first
	}

	for {
		select {
		case <-q.ctx.Done():
			return nil
		case size := <-q.sizes:
			// A zero dimension would collapse the PTY, so it is skipped rather than sent.
			if size.Cols == 0 || size.Rows == 0 {
				continue
			}
			return &remotecommand.TerminalSize{Width: size.Cols, Height: size.Rows}
		}
	}
}
