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

// GatewayInfo is the flattened gateway representation rendered by the table.
type GatewayInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Class      string `json:"class"`
	Address    string `json:"address"`
	Programmed string `json:"programmed"`
	Age        string `json:"age"`
}

// WatchGateways streams gateway snapshots for one client, across all namespaces. It returns when
// ctx is done, or when the probe or the first cache sync fails.
func WatchGateways(
	ctx context.Context,
	clientset gatewayclient.Interface,
	onSnapshot func([]GatewayInfo),
) error {
	factory := gatewayinformers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Gateway",
		factory,
		factory.Gateway().V1().Gateways().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.GatewayV1().Gateways("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return gatewayAPIError(err)
		},
		gatewaysFromStore,
		onSnapshot,
	)
}

func gatewaysFromStore(store cache.Store) []GatewayInfo {
	now := time.Now()
	objects := store.List()
	gateways := make([]GatewayInfo, 0, len(objects))

	for _, object := range objects {
		gateway, ok := object.(*gatewayv1.Gateway)
		if !ok {
			continue
		}
		gateways = append(gateways, toGatewayInfo(gateway, now))
	}

	sortByNamespaceAndName(
		gateways,
		func(gateway GatewayInfo) string { return gateway.Namespace },
		func(gateway GatewayInfo) string { return gateway.Name },
	)

	return gateways
}

// toGatewayInfo flattens a gateway the way `kubectl get gateway -A` presents it: CLASS is the
// gateway class it binds to, ADDRESS what the controller bound, and PROGRAMMED whether the
// controller finished reconciling it.
func toGatewayInfo(gateway *gatewayv1.Gateway, now time.Time) GatewayInfo {
	return GatewayInfo{
		Namespace:  gateway.Namespace,
		Name:       gateway.Name,
		Class:      string(gateway.Spec.GatewayClassName),
		Address:    gatewayAddresses(gateway.Status),
		Programmed: conditionStatus(gateway.Status.Conditions, "Programmed"),
		Age:        duration.HumanDuration(now.Sub(gateway.CreationTimestamp.Time)),
	}
}

// GatewayYAML renders one gateway the way `kubectl get gateway -o yaml` prints it. Like PodYAML
// the type fields are set by hand, because the typed client leaves them empty, and managedFields
// is dropped the way kubectl hides that bookkeeping by default.
func GatewayYAML(ctx context.Context, clientset gatewayclient.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	gateway, err := clientset.GatewayV1().Gateways(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read Gateway %s/%s: %w", namespace, name, err)
	}

	gateway.APIVersion = gatewayAPIGroupVersion
	gateway.Kind = "Gateway"
	gateway.ManagedFields = nil

	document, err := yaml.Marshal(gateway)
	if err != nil {
		return "", fmt.Errorf("cannot encode Gateway %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}
