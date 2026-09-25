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

// HTTPRouteInfo is the flattened HTTP route representation rendered by the table.
type HTTPRouteInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Hostnames  string `json:"hostnames"`
	ParentRefs string `json:"parentRefs"`
	Age        string `json:"age"`
}

// WatchHTTPRoutes streams HTTP route snapshots for one client, across all namespaces. It returns
// when ctx is done, or when the probe or the first cache sync fails.
func WatchHTTPRoutes(
	ctx context.Context,
	clientset gatewayclient.Interface,
	onSnapshot func([]HTTPRouteInfo),
) error {
	factory := gatewayinformers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"HTTPRoute",
		factory,
		factory.Gateway().V1().HTTPRoutes().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.GatewayV1().HTTPRoutes("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return gatewayAPIError(err)
		},
		httpRoutesFromStore,
		onSnapshot,
	)
}

func httpRoutesFromStore(store cache.Store) []HTTPRouteInfo {
	now := time.Now()
	objects := store.List()
	routes := make([]HTTPRouteInfo, 0, len(objects))

	for _, object := range objects {
		route, ok := object.(*gatewayv1.HTTPRoute)
		if !ok {
			continue
		}
		routes = append(routes, toHTTPRouteInfo(route, now))
	}

	sortByNamespaceAndName(
		routes,
		func(route HTTPRouteInfo) string { return route.Namespace },
		func(route HTTPRouteInfo) string { return route.Name },
	)

	return routes
}

// toHTTPRouteInfo flattens an HTTP route the way `kubectl get httproute -A` presents it.
func toHTTPRouteInfo(route *gatewayv1.HTTPRoute, now time.Time) HTTPRouteInfo {
	return HTTPRouteInfo{
		Namespace:  route.Namespace,
		Name:       route.Name,
		Hostnames:  routeHostnames(route.Spec.Hostnames),
		ParentRefs: routeParentRefs(route.Spec.ParentRefs, route.Namespace),
		Age:        duration.HumanDuration(now.Sub(route.CreationTimestamp.Time)),
	}
}

// HTTPRouteYAML renders one HTTP route the way `kubectl get httproute -o yaml` prints it. Like
// PodYAML the type fields are set by hand, because the typed client leaves them empty, and
// managedFields is dropped the way kubectl hides that bookkeeping by default.
func HTTPRouteYAML(ctx context.Context, clientset gatewayclient.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	route, err := clientset.GatewayV1().HTTPRoutes(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read HTTPRoute %s/%s: %w", namespace, name, err)
	}

	route.APIVersion = gatewayAPIGroupVersion
	route.Kind = "HTTPRoute"
	route.ManagedFields = nil

	document, err := yaml.Marshal(route)
	if err != nil {
		return "", fmt.Errorf("cannot encode HTTPRoute %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}
