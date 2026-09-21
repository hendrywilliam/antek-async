package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func runningContainer(name string, ready bool, restarts int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:         name,
		Ready:        ready,
		RestartCount: restarts,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
}

func waitingContainer(name, reason string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  name,
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
	}
}

func terminatedContainer(name, reason string, exitCode int32, signal int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name: name,
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason:   reason,
				ExitCode: exitCode,
				Signal:   signal,
			},
		},
	}
}

func pod(status corev1.PodStatus, containers ...corev1.Container) *corev1.Pod {
	names := make([]corev1.Container, 0, len(containers))
	for _, container := range containers {
		names = append(names, container)
	}

	return &corev1.Pod{
		Spec:   corev1.PodSpec{Containers: names},
		Status: status,
	}
}

func readyCondition() corev1.PodCondition {
	return corev1.PodCondition{Type: corev1.PodReady, Status: corev1.ConditionTrue}
}

func TestPodStatus(t *testing.T) {
	deleted := metav1.NewTime(time.Now())

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "running pod with ready containers",
			pod: pod(corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{runningContainer("app", true, 0)},
			}),
			want: "Running",
		},
		{
			name: "waiting reason wins over phase",
			pod: pod(corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{waitingContainer("app", "CrashLoopBackOff")},
			}),
			want: "CrashLoopBackOff",
		},
		{
			name: "pending pod without container statuses",
			pod:  pod(corev1.PodStatus{Phase: corev1.PodPending}),
			want: "Pending",
		},
		{
			name: "status reason wins over phase",
			pod: pod(corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "Evicted",
			}),
			want: "Evicted",
		},
		{
			name: "completed pod stays completed",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("app", "Completed", 0, 0),
				},
			}),
			want: "Completed",
		},
		{
			name: "completed pod with a running container is running again",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				Conditions: []corev1.PodCondition{
					readyCondition(),
				},
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("done", "Completed", 0, 0),
					runningContainer("app", true, 0),
				},
			}),
			want: "Running",
		},
		{
			name: "completed pod with a ready running container needs the ready condition",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("done", "Completed", 0, 0),
					runningContainer("app", true, 0),
				},
			}),
			want: "NotReady",
		},
		{
			name: "completed pod whose running container is not ready stays completed",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("done", "Completed", 0, 0),
					runningContainer("app", false, 0),
				},
			}),
			want: "Completed",
		},
		{
			name: "succeeded init container falls through to containers",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase: corev1.PodRunning,
					InitContainerStatuses: []corev1.ContainerStatus{
						terminatedContainer("init", "Completed", 0, 0),
					},
					ContainerStatuses: []corev1.ContainerStatus{runningContainer("app", true, 0)},
				})
				p.Spec.InitContainers = []corev1.Container{{Name: "init"}}

				return p
			}(),
			want: "Running",
		},
		{
			name: "failing init container is prefixed",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						waitingContainer("init", "CrashLoopBackOff"),
					},
				})
				p.Spec.InitContainers = []corev1.Container{{Name: "init"}}

				return p
			}(),
			want: "Init:CrashLoopBackOff",
		},
		{
			name: "init container terminated with reason",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						terminatedContainer("init", "Error", 1, 0),
					},
				})
				p.Spec.InitContainers = []corev1.Container{{Name: "init"}}

				return p
			}(),
			want: "Init:Error",
		},
		{
			name: "init container terminated without reason reports exit code",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						terminatedContainer("init", "", 2, 0),
					},
				})
				p.Spec.InitContainers = []corev1.Container{{Name: "init"}}

				return p
			}(),
			want: "Init:ExitCode:2",
		},
		{
			name: "container terminated without reason reports exit code",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("app", "", 1, 0),
				},
			}),
			want: "ExitCode:1",
		},
		{
			name: "container terminated by signal reports signal",
			pod: pod(corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					terminatedContainer("app", "", 137, 9),
				},
			}),
			want: "Signal:9",
		},
		{
			name: "pod being deleted is terminating",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase:             corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{runningContainer("app", true, 0)},
				})
				p.DeletionTimestamp = &deleted

				return p
			}(),
			want: "Terminating",
		},
		{
			name: "pod lost with a deletion timestamp is unknown",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase:  corev1.PodRunning,
					Reason: "NodeLost",
				})
				p.DeletionTimestamp = &deleted

				return p
			}(),
			want: "Unknown",
		},
		{
			name: "init container still running reports progress",
			pod: func() *corev1.Pod {
				p := pod(corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  "init",
							State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						},
					},
				})
				p.Spec.InitContainers = []corev1.Container{{Name: "init"}}

				return p
			}(),
			want: "Init:0/1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := podStatus(test.pod); got != test.want {
				t.Errorf("podStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToPodInfo(t *testing.T) {
	now := time.Now()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "production",
			Name:              "api-7d9f",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			NodeName: "worker-1",
			Containers: []corev1.Container{
				{Name: "api"},
				{Name: "sidecar"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				runningContainer("api", true, 2),
				runningContainer("sidecar", false, 3),
			},
		},
	}

	got := toPodInfo(pod, now)

	if got.Namespace != "production" || got.Name != "api-7d9f" {
		t.Errorf("namespace/name = %q/%q, want production/api-7d9f", got.Namespace, got.Name)
	}
	if got.Ready != "1/2" {
		t.Errorf("ready = %q, want 1/2", got.Ready)
	}
	if got.Restarts != 5 {
		t.Errorf("restarts = %d, want 5", got.Restarts)
	}
	if got.Status != "Running" {
		t.Errorf("status = %q, want Running", got.Status)
	}
	if got.Node != "worker-1" {
		t.Errorf("node = %q, want worker-1", got.Node)
	}
	if got.Age != "5m" {
		t.Errorf("age = %q, want 5m", got.Age)
	}

	unscheduled := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}
	if info := toPodInfo(unscheduled, now); info.Node != "-" {
		t.Errorf("node for unscheduled pod = %q, want -", info.Node)
	} else if info.Ready != "0/0" {
		t.Errorf("ready for pod without containers = %q, want 0/0", info.Ready)
	}
}

func TestPodsFromStoreSortsByNameAndNamespace(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, ref := range []struct{ namespace, name string }{
		{"default", "zebra"},
		{"kube-system", "alpha"},
		{"default", "alpha"},
		{"kube-system", "zebra"},
	} {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ref.namespace, Name: ref.name}}
		if err := store.Add(pod); err != nil {
			t.Fatalf("add %s/%s: %v", ref.namespace, ref.name, err)
		}
	}

	// Objects of another type must be skipped instead of breaking the snapshot.
	if err := store.Add(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add namespace: %v", err)
	}

	pods := podsFromStore(store)

	want := []string{
		"default/alpha",
		"default/zebra",
		"kube-system/alpha",
		"kube-system/zebra",
	}
	if len(pods) != len(want) {
		t.Fatalf("got %d pods, want %d", len(pods), len(want))
	}
	for i, key := range want {
		if got := pods[i].Namespace + "/" + pods[i].Name; got != key {
			t.Errorf("pod %d = %q, want %q", i, got, key)
		}
	}
}
