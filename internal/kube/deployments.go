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

// DeploymentInfo is the flattened deployment representation rendered by the table.
type DeploymentInfo struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	UpToDate  int32  `json:"upToDate"`
	Available int32  `json:"available"`
	Age       string `json:"age"`
}

// WatchDeployments streams deployment snapshots for one client, across all namespaces. It
// returns when ctx is done, or when the probe or the first cache sync fails.
func WatchDeployments(
	ctx context.Context,
	clientset kubernetes.Interface,
	onSnapshot func([]DeploymentInfo),
) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Deployment",
		factory,
		factory.Apps().V1().Deployments().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.AppsV1().Deployments("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		deploymentsFromStore,
		onSnapshot,
	)
}

func deploymentsFromStore(store cache.Store) []DeploymentInfo {
	now := time.Now()
	objects := store.List()
	deployments := make([]DeploymentInfo, 0, len(objects))

	for _, object := range objects {
		deployment, ok := object.(*appsv1.Deployment)
		if !ok {
			continue
		}
		deployments = append(deployments, toDeploymentInfo(deployment, now))
	}

	sortByNamespaceAndName(
		deployments,
		func(deployment DeploymentInfo) string { return deployment.Namespace },
		func(deployment DeploymentInfo) string { return deployment.Name },
	)

	return deployments
}

// toDeploymentInfo flattens a deployment the way `kubectl get deployments -A` presents it:
// READY, UP-TO-DATE and AVAILABLE come from the status counters.
func toDeploymentInfo(deployment *appsv1.Deployment, now time.Time) DeploymentInfo {
	return DeploymentInfo{
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
		Ready:     replicaCount(deployment.Status.ReadyReplicas, deployment.Spec.Replicas),
		UpToDate:  deployment.Status.UpdatedReplicas,
		Available: deployment.Status.AvailableReplicas,
		Age:       duration.HumanDuration(now.Sub(deployment.CreationTimestamp.Time)),
	}
}
