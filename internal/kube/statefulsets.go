package kube

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// StatefulSetInfo is the flattened statefulset representation rendered by the table.
type StatefulSetInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	Age       string `json:"age"`
}

// WatchStatefulSets streams statefulset snapshots for one client, across all namespaces. It
// returns when ctx is done, or when the probe or the first cache sync fails.
func WatchStatefulSets(
	ctx context.Context,
	clientset kubernetes.Interface,
	onSnapshot func([]StatefulSetInfo),
) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"StatefulSet",
		factory,
		factory.Apps().V1().StatefulSets().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.AppsV1().StatefulSets("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		statefulSetsFromStore,
		onSnapshot,
	)
}

func statefulSetsFromStore(store cache.Store) []StatefulSetInfo {
	now := time.Now()
	objects := store.List()
	statefulSets := make([]StatefulSetInfo, 0, len(objects))

	for _, object := range objects {
		statefulSet, ok := object.(*appsv1.StatefulSet)
		if !ok {
			continue
		}
		statefulSets = append(statefulSets, toStatefulSetInfo(statefulSet, now))
	}

	sortByNamespaceAndName(
		statefulSets,
		func(statefulSet StatefulSetInfo) string { return statefulSet.Namespace },
		func(statefulSet StatefulSetInfo) string { return statefulSet.Name },
	)

	return statefulSets
}

// toStatefulSetInfo flattens a statefulset the way `kubectl get statefulsets -A` presents it.
func toStatefulSetInfo(statefulSet *appsv1.StatefulSet, now time.Time) StatefulSetInfo {
	return StatefulSetInfo{
		Namespace: statefulSet.Namespace,
		Name:      statefulSet.Name,
		Ready:     replicaCount(statefulSet.Status.ReadyReplicas, statefulSet.Spec.Replicas),
		Age:       duration.HumanDuration(now.Sub(statefulSet.CreationTimestamp.Time)),
	}
}
