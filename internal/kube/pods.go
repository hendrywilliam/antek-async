package kube

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	yaml "sigs.k8s.io/yaml"
)

type PodInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
	IP        string `json:"ip"`
	Node      string `json:"node"`
}

// WatchPods streams pod snapshots for one client, across all namespaces. It returns when ctx
// is done, or when the probe or the first cache sync fails.
func WatchPods(ctx context.Context, clientset kubernetes.Interface, onSnapshot func([]PodInfo)) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Pod",
		factory,
		factory.Core().V1().Pods().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.CoreV1().Pods("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		podsFromStore,
		onSnapshot,
	)
}

func podsFromStore(store cache.Store) []PodInfo {
	now := time.Now()
	objects := store.List()
	pods := make([]PodInfo, 0, len(objects))

	for _, object := range objects {
		pod, ok := object.(*corev1.Pod)
		if !ok {
			continue
		}
		pods = append(pods, toPodInfo(pod, now))
	}

	sortByNamespaceAndName(
		pods,
		func(pod PodInfo) string { return pod.Namespace },
		func(pod PodInfo) string { return pod.Name },
	)

	return pods
}

// PodYAML renders one pod the way `kubectl get pod -o yaml` prints it. The type fields are set
// by hand because the typed client leaves them empty, and managedFields is dropped because
// kubectl hides that bookkeeping by default.
func PodYAML(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	pod, err := clientset.CoreV1().Pods(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read Pod %s/%s: %w", namespace, name, err)
	}

	pod.APIVersion = "v1"
	pod.Kind = "Pod"
	pod.ManagedFields = nil

	document, err := yaml.Marshal(pod)
	if err != nil {
		return "", fmt.Errorf("cannot encode Pod %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}

func toPodInfo(pod *corev1.Pod, now time.Time) PodInfo {
	ready := 0
	var restarts int32
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}

	node := pod.Spec.NodeName
	if node == "" {
		node = "-"
	}

	// A pod without an address yet, which is the case while it is still pending.
	ip := pod.Status.PodIP
	if ip == "" {
		ip = "-"
	}

	return PodInfo{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Ready:     fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
		Status:    podStatus(pod),
		Restarts:  restarts,
		Age:       duration.HumanDuration(now.Sub(pod.CreationTimestamp.Time)),
		IP:        ip,
		Node:      node,
	}
}

func podStatus(pod *corev1.Pod) string {
	reason := string(pod.Status.Phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	initializing := false
	for i := range pod.Status.InitContainerStatuses {
		status := pod.Status.InitContainerStatuses[i]
		switch {
		case status.State.Terminated != nil && status.State.Terminated.ExitCode == 0:
			continue
		case status.State.Terminated != nil:
			switch {
			case status.State.Terminated.Reason != "":
				reason = "Init:" + status.State.Terminated.Reason
			case status.State.Terminated.Signal != 0:
				reason = fmt.Sprintf("Init:Signal:%d", status.State.Terminated.Signal)
			default:
				reason = fmt.Sprintf("Init:ExitCode:%d", status.State.Terminated.ExitCode)
			}
			initializing = true
		case status.State.Waiting != nil && status.State.Waiting.Reason != "" && status.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + status.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, len(pod.Spec.InitContainers))
			initializing = true
		}
		break
	}

	if !initializing || podInitialized(pod.Status) {
		hasRunning := false
		for i := len(pod.Status.ContainerStatuses) - 1; i >= 0; i-- {
			status := pod.Status.ContainerStatuses[i]
			switch {
			case status.State.Waiting != nil && status.State.Waiting.Reason != "":
				reason = status.State.Waiting.Reason
			case status.State.Terminated != nil && status.State.Terminated.Reason != "":
				reason = status.State.Terminated.Reason
			case status.State.Terminated != nil && status.State.Terminated.Signal != 0:
				reason = fmt.Sprintf("Signal:%d", status.State.Terminated.Signal)
			case status.State.Terminated != nil:
				reason = fmt.Sprintf("ExitCode:%d", status.State.Terminated.ExitCode)
			case status.Ready && status.State.Running != nil:
				hasRunning = true
			}
		}

		if reason == "Completed" && hasRunning {
			if podReadyCondition(pod.Status) {
				reason = "Running"
			} else {
				reason = "NotReady"
			}
		}
	}

	switch {
	case pod.DeletionTimestamp != nil && pod.Status.Reason == "NodeLost":
		reason = "Unknown"
	case pod.DeletionTimestamp != nil:
		reason = "Terminating"
	}

	return reason
}

func podInitialized(status corev1.PodStatus) bool {
	for _, condition := range status.Conditions {
		if condition.Type == corev1.PodInitialized {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podReadyCondition(status corev1.PodStatus) bool {
	for _, condition := range status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
