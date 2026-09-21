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
)

// NamespaceInfo is the flattened namespace representation rendered by the table.
type NamespaceInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

// WatchNamespaces streams namespace snapshots for one client. Namespaces are cluster scoped, so
// there is no namespace to watch. It returns when ctx is done, or when the probe or the first
// cache sync fails.
func WatchNamespaces(ctx context.Context, clientset kubernetes.Interface, onSnapshot func([]NamespaceInfo)) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Namespace",
		factory,
		factory.Core().V1().Namespaces().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.CoreV1().Namespaces().List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		namespacesFromStore,
		onSnapshot,
	)
}

func namespacesFromStore(store cache.Store) []NamespaceInfo {
	now := time.Now()
	objects := store.List()
	namespaces := make([]NamespaceInfo, 0, len(objects))

	for _, object := range objects {
		namespace, ok := object.(*corev1.Namespace)
		if !ok {
			continue
		}
		namespaces = append(namespaces, toNamespaceInfo(namespace, now))
	}

	sortByName(namespaces, func(namespace NamespaceInfo) string { return namespace.Name })

	return namespaces
}

// toNamespaceInfo flattens a namespace the way `kubectl get namespaces` presents it. The
// cluster-scoped NAME is the namespace itself, so there is no namespace column.
// DeleteNamespace removes one namespace the way `kubectl delete namespace` does. The API server
// then runs the namespace's own finalizers, so a namespace that takes time to go drains in the
// watch as Terminating before it disappears.
func DeleteNamespace(ctx context.Context, clientset kubernetes.Interface, name string) error {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	if err := clientset.CoreV1().Namespaces().Delete(requestCtx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("cannot delete Namespace %s: %w", name, err)
	}

	return nil
}

func toNamespaceInfo(namespace *corev1.Namespace, now time.Time) NamespaceInfo {
	return NamespaceInfo{
		Name:   namespace.Name,
		Status: string(namespace.Status.Phase),
		Age:    duration.HumanDuration(now.Sub(namespace.CreationTimestamp.Time)),
	}
}
