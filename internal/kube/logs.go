package kube

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// LogRequest names one log stream: the pod, the container to read from, and whether to keep
// following after the history is sent. TailLines caps that history, and a zero value means the
// whole log rather than no lines at all.
type LogRequest struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Follow    bool   `json:"follow"`
	TailLines int64  `json:"tailLines"`
}

// StreamPodLogs streams one container's log to streams.Stdout until ctx is done or the container
// stops writing. It builds its own client from the kubeconfig path for the same reason
// ExecTerminal does: the request is not tied to whichever watch happens to be running.
func StreamPodLogs(ctx context.Context, path string, request LogRequest, streams TermStreams) error {
	restConfig, err := RestConfigFor(path)
	if err != nil {
		return err
	}

	// restConfig.Timeout stays unset the way RestConfigFor leaves it: it also applies to this
	// stream, and a followed log lives until the viewer closes rather than for one interval.
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	return PodLogs(ctx, clientset, request, streams.Stdout)
}

// PodLogs opens the log endpoint of one container and copies it to out. It takes the client rather
// than a path so the endpoint, the options and the error wording can be exercised against a test
// server without a cluster.
func PodLogs(ctx context.Context, clientset kubernetes.Interface, request LogRequest, out io.Writer) error {
	stream, err := clientset.CoreV1().Pods(request.Namespace).
		GetLogs(request.Pod, podLogOptions(request)).
		Stream(ctx)
	if err != nil {
		return fmt.Errorf("cannot read logs for Pod %s/%s: %w", request.Namespace, request.Pod, err)
	}
	defer stream.Close()

	_, err = io.Copy(out, stream)

	return err
}

// podLogOptions is what the API server reads the log with. A zero TailLines is left nil rather
// than sent as zero, because the API server reads a missing value as the whole log and an explicit
// zero as no lines at all.
func podLogOptions(request LogRequest) *corev1.PodLogOptions {
	options := &corev1.PodLogOptions{
		Container: request.Container,
		Follow:    request.Follow,
	}

	if request.TailLines > 0 {
		lines := request.TailLines
		options.TailLines = &lines
	}

	return options
}
