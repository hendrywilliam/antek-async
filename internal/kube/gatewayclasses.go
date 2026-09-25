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

// GatewayClassInfo is the flattened gateway class representation rendered by the table.
type GatewayClassInfo struct {
	Name       string `json:"name"`
	Controller string `json:"controller"`
	Accepted   string `json:"accepted"`
	Age        string `json:"age"`
}

// WatchGatewayClasses streams gateway class snapshots for one client. Like nodes and namespaces
// it is cluster scoped, so neither the probe nor the informer takes a namespace. It returns when
// ctx is done, or when the probe or the first cache sync fails.
func WatchGatewayClasses(
	ctx context.Context,
	clientset gatewayclient.Interface,
	onSnapshot func([]GatewayClassInfo),
) error {
	factory := gatewayinformers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"GatewayClass",
		factory,
		factory.Gateway().V1().GatewayClasses().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.GatewayV1().GatewayClasses().List(probeCtx, metav1.ListOptions{Limit: 1})
			return gatewayAPIError(err)
		},
		gatewayClassesFromStore,
		onSnapshot,
	)
}

func gatewayClassesFromStore(store cache.Store) []GatewayClassInfo {
	now := time.Now()
	objects := store.List()
	classes := make([]GatewayClassInfo, 0, len(objects))

	for _, object := range objects {
		class, ok := object.(*gatewayv1.GatewayClass)
		if !ok {
			continue
		}
		classes = append(classes, toGatewayClassInfo(class, now))
	}

	sortByName(classes, func(class GatewayClassInfo) string { return class.Name })

	return classes
}

// toGatewayClassInfo flattens a gateway class the way `kubectl get gatewayclass` presents it:
// CONTROLLER is the controller that owns the class, and ACCEPTED is that controller's verdict.
func toGatewayClassInfo(class *gatewayv1.GatewayClass, now time.Time) GatewayClassInfo {
	return GatewayClassInfo{
		Name:       class.Name,
		Controller: string(class.Spec.ControllerName),
		Accepted:   conditionStatus(class.Status.Conditions, "Accepted"),
		Age:        duration.HumanDuration(now.Sub(class.CreationTimestamp.Time)),
	}
}

// GatewayClassYAML renders one gateway class the way `kubectl get gatewayclass -o yaml` prints
// it. Like PodYAML the type fields are set by hand, because the typed client leaves them empty,
// and managedFields is dropped the way kubectl hides that bookkeeping by default.
func GatewayClassYAML(ctx context.Context, clientset gatewayclient.Interface, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	class, err := clientset.GatewayV1().GatewayClasses().Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read GatewayClass %s: %w", name, err)
	}

	class.APIVersion = gatewayAPIGroupVersion
	class.Kind = "GatewayClass"
	class.ManagedFields = nil

	document, err := yaml.Marshal(class)
	if err != nil {
		return "", fmt.Errorf("cannot encode GatewayClass %s: %w", name, err)
	}

	return string(document), nil
}
