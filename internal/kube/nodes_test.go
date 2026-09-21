package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func nodeWithConditions(conditions ...corev1.NodeCondition) *corev1.Node {
	return &corev1.Node{Status: corev1.NodeStatus{Conditions: conditions}}
}

func nodeReadyCondition(status corev1.ConditionStatus) corev1.NodeCondition {
	return corev1.NodeCondition{Type: corev1.NodeReady, Status: status}
}

func TestNodeStatus(t *testing.T) {
	cordoned := nodeWithConditions(nodeReadyCondition(corev1.ConditionTrue))
	cordoned.Spec.Unschedulable = true

	tests := []struct {
		name string
		node *corev1.Node
		want string
	}{
		{
			name: "ready node",
			node: nodeWithConditions(nodeReadyCondition(corev1.ConditionTrue)),
			want: "Ready",
		},
		{
			name: "not ready node",
			node: nodeWithConditions(nodeReadyCondition(corev1.ConditionFalse)),
			want: "NotReady",
		},
		{
			name: "unknown ready condition",
			node: nodeWithConditions(nodeReadyCondition(corev1.ConditionUnknown)),
			want: "Unknown",
		},
		{
			name: "node without the ready condition",
			node: nodeWithConditions(),
			want: "Unknown",
		},
		{
			name: "cordoned node wins over the ready condition",
			node: cordoned,
			want: "SchedulingDisabled",
		},
		{
			name: "other conditions are ignored",
			node: nodeWithConditions(
				corev1.NodeCondition{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				nodeReadyCondition(corev1.ConditionTrue),
			),
			want: "Ready",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nodeStatus(test.node); got != test.want {
				t.Errorf("nodeStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNodeRoles(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "no roles",
			labels: map[string]string{"kubernetes.io/hostname": "node-1"},
			want:   "<none>",
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   "<none>",
		},
		{
			name: "control plane roles are sorted and joined",
			labels: map[string]string{
				"node-role.kubernetes.io/master":        "",
				"node-role.kubernetes.io/control-plane": "",
				"kubernetes.io/hostname":                "node-1",
			},
			want: "control-plane,master",
		},
		{
			name:   "legacy role label is honoured",
			labels: map[string]string{"kubernetes.io/role": "worker"},
			want:   "worker",
		},
		{
			name: "prefix and legacy labels combine",
			labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
				"kubernetes.io/role":             "node",
			},
			want: "node,worker",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: test.labels}}
			if got := nodeRoles(node); got != test.want {
				t.Errorf("nodeRoles() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToNodeInfo(t *testing.T) {
	now := time.Now()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "master-1",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
			Labels:            map[string]string{roleLabelPrefix + "control-plane": ""},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{nodeReadyCondition(corev1.ConditionTrue)},
			NodeInfo:   corev1.NodeSystemInfo{KubeletVersion: "v1.31.5+k3s1"},
		},
	}

	got := toNodeInfo(node, now)

	if got.Name != "master-1" {
		t.Errorf("name = %q, want master-1", got.Name)
	}
	if got.Status != "Ready" {
		t.Errorf("status = %q, want Ready", got.Status)
	}
	if got.Roles != "control-plane" {
		t.Errorf("roles = %q, want control-plane", got.Roles)
	}
	if got.Version != "v1.31.5+k3s1" {
		t.Errorf("version = %q, want v1.31.5+k3s1", got.Version)
	}
	if got.Age != "5h" {
		t.Errorf("age = %q, want 5h", got.Age)
	}
}

func TestNodesFromStoreSortsByName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, name := range []string{"worker-2", "master-1", "worker-1"} {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := store.Add(node); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add pod: %v", err)
	}

	got := nodesFromStore(store)

	want := []string{"master-1", "worker-1", "worker-2"}
	if len(got) != len(want) {
		t.Fatalf("got %d nodes, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("node %d = %q, want %q", i, got[i].Name, name)
		}
	}
}
