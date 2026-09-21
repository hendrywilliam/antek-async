package kube

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	probeTimeout    = 15 * time.Second
	flushInterval   = time.Second
	ageRefreshEvery = 10 * time.Second
)

type PodInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restarts"`
	Age       string `json:"age"`
	Node      string `json:"node"`
}

func Watch(ctx context.Context, path string, onSnapshot func([]PodInfo)) error {
	clientset, err := clientFor(path)
	if err != nil {
		return err
	}

	probeCtx, cancelProbe := context.WithTimeout(ctx, probeTimeout)
	_, probeErr := clientset.CoreV1().Pods("").List(probeCtx, metav1.ListOptions{Limit: 1})
	cancelProbe()
	if probeErr != nil {
		return probeErr
	}

	factory := informers.NewSharedInformerFactory(clientset, 0)
	informer := factory.Core().V1().Pods().Informer()

	var dirty atomic.Bool
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { dirty.Store(true) },
		UpdateFunc: func(any, any) { dirty.Store(true) },
		DeleteFunc: func(any) { dirty.Store(true) },
	}); err != nil {
		return err
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New("gagal sinkronisasi cache Pod")
	}

	onSnapshot(podsFromStore(informer.GetStore()))

	flush := time.NewTicker(flushInterval)
	defer flush.Stop()
	refreshAge := time.NewTicker(ageRefreshEvery)
	defer refreshAge.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-refreshAge.C:
			dirty.Store(true)
		case <-flush.C:
			if dirty.Swap(false) {
				onSnapshot(podsFromStore(informer.GetStore()))
			}
		}
	}
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

	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})

	return pods
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

	return PodInfo{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Ready:     fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
		Status:    podStatus(pod),
		Restarts:  restarts,
		Age:       duration.HumanDuration(now.Sub(pod.CreationTimestamp.Time)),
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
