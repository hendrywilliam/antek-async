package kube

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gatewayinformers "sigs.k8s.io/gateway-api/pkg/client/informers/externalversions"
	yaml "sigs.k8s.io/yaml"
)

// GRPCRouteInfo is the flattened gRPC route representation rendered by the table.
type GRPCRouteInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Hostnames  string `json:"hostnames"`
	ParentRefs string `json:"parentRefs"`
	Age        string `json:"age"`
}

// WatchGRPCRoutes streams gRPC route snapshots for one client, across all namespaces. It returns
// when ctx is done, or when the probe or the first cache sync fails.
func WatchGRPCRoutes(
	ctx context.Context,
	clientset gatewayclient.Interface,
	onSnapshot func([]GRPCRouteInfo),
) error {
	factory := gatewayinformers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"GRPCRoute",
		factory,
		factory.Gateway().V1().GRPCRoutes().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.GatewayV1().GRPCRoutes("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return gatewayAPIError(err)
		},
		grpcRoutesFromStore,
		onSnapshot,
	)
}

func grpcRoutesFromStore(store cache.Store) []GRPCRouteInfo {
	now := time.Now()
	objects := store.List()
	routes := make([]GRPCRouteInfo, 0, len(objects))

	for _, object := range objects {
		route, ok := object.(*gatewayv1.GRPCRoute)
		if !ok {
			continue
		}
		routes = append(routes, toGRPCRouteInfo(route, now))
	}

	sortByNamespaceAndName(
		routes,
		func(route GRPCRouteInfo) string { return route.Namespace },
		func(route GRPCRouteInfo) string { return route.Name },
	)

	return routes
}

// toGRPCRouteInfo flattens a gRPC route the way `kubectl get grpcroute -A` presents it.
func toGRPCRouteInfo(route *gatewayv1.GRPCRoute, now time.Time) GRPCRouteInfo {
	return GRPCRouteInfo{
		Namespace:  route.Namespace,
		Name:       route.Name,
		Hostnames:  routeHostnames(route.Spec.Hostnames),
		ParentRefs: routeParentRefs(route.Spec.ParentRefs, route.Namespace),
		Age:        duration.HumanDuration(now.Sub(route.CreationTimestamp.Time)),
	}
}

// GRPCRouteYAML renders one gRPC route the way `kubectl get grpcroute -o yaml` prints it. Like
// PodYAML the type fields are set by hand and managedFields is dropped.
func GRPCRouteYAML(ctx context.Context, clientset gatewayclient.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	route, err := clientset.GatewayV1().GRPCRoutes(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read GRPCRoute %s/%s: %w", namespace, name, err)
	}

	route.APIVersion = gatewayAPIGroupVersion
	route.Kind = "GRPCRoute"
	route.ManagedFields = nil

	document, err := yaml.Marshal(route)
	if err != nil {
		return "", fmt.Errorf("cannot encode GRPCRoute %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}
