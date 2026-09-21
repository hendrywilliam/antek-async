package kube

import (
	"context"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	// roleLabelPrefix is the label prefix kubectl reads to build the ROLES column.
	roleLabelPrefix = "node-role.kubernetes.io/"
	// legacyRoleLabel is the older single-role label kubectl still honours.
	legacyRoleLabel = "kubernetes.io/role"
)

// NodeInfo is the flattened node representation rendered by the table.
type NodeInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Roles   string `json:"roles"`
	Version string `json:"version"`
	Age     string `json:"age"`
}

// WatchNodes streams node snapshots for one client. Nodes are cluster scoped, so there is no
// namespace to watch. It returns when ctx is done, or when the probe or the first cache sync
// fails.
func WatchNodes(ctx context.Context, clientset kubernetes.Interface, onSnapshot func([]NodeInfo)) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Node",
		factory,
		factory.Core().V1().Nodes().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.CoreV1().Nodes().List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		nodesFromStore,
		onSnapshot,
	)
}

func nodesFromStore(store cache.Store) []NodeInfo {
	now := time.Now()
	objects := store.List()
	nodes := make([]NodeInfo, 0, len(objects))

	for _, object := range objects {
		node, ok := object.(*corev1.Node)
		if !ok {
			continue
		}
		nodes = append(nodes, toNodeInfo(node, now))
	}

	sortByName(nodes, func(node NodeInfo) string { return node.Name })

	return nodes
}

// toNodeInfo flattens a node the way `kubectl get nodes` presents it.
func toNodeInfo(node *corev1.Node, now time.Time) NodeInfo {
	return NodeInfo{
		Name:    node.Name,
		Status:  nodeStatus(node),
		Roles:   nodeRoles(node),
		Version: node.Status.NodeInfo.KubeletVersion,
		Age:     duration.HumanDuration(now.Sub(node.CreationTimestamp.Time)),
	}
}

// nodeStatus mirrors kubectl's STATUS column: a cordoned node reports SchedulingDisabled,
// otherwise the Ready condition decides, and a node without that condition is Unknown.
func nodeStatus(node *corev1.Node) string {
	if node.Spec.Unschedulable {
		return "SchedulingDisabled"
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type != corev1.NodeReady {
			continue
		}

		switch condition.Status {
		case corev1.ConditionTrue:
			return "Ready"
		case corev1.ConditionFalse:
			return "NotReady"
		case corev1.ConditionUnknown:
			return "Unknown"
		}
	}

	return "Unknown"
}

// nodeRoles mirrors kubectl's ROLES column: every label under the role prefix counts, the
// legacy label still does, the list is sorted, and a node without roles reads "<none>".
func nodeRoles(node *corev1.Node) string {
	roles := make([]string, 0, len(node.Labels))

	for label := range node.Labels {
		if strings.HasPrefix(label, roleLabelPrefix) {
			roles = append(roles, strings.TrimPrefix(label, roleLabelPrefix))
		}
	}

	if legacy, ok := node.Labels[legacyRoleLabel]; ok {
		roles = append(roles, legacy)
	}

	if len(roles) == 0 {
		return "<none>"
	}

	sort.Strings(roles)

	return strings.Join(roles, ",")
}
