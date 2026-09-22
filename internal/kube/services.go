package kube

import (
	"context"
	"fmt"
	"slices"
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

// ServiceInfo is the flattened service representation rendered by the table.
type ServiceInfo struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	ClusterIP  string `json:"clusterIP"`
	ExternalIP string `json:"externalIP"`
	Ports      string `json:"ports"`
	Age        string `json:"age"`
}

// WatchServices streams service snapshots for one client, across all namespaces. It returns when
// ctx is done, or when the probe or the first cache sync fails.
func WatchServices(
	ctx context.Context,
	clientset kubernetes.Interface,
	onSnapshot func([]ServiceInfo),
) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"Service",
		factory,
		factory.Core().V1().Services().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.CoreV1().Services("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		servicesFromStore,
		onSnapshot,
	)
}

func servicesFromStore(store cache.Store) []ServiceInfo {
	now := time.Now()
	objects := store.List()
	services := make([]ServiceInfo, 0, len(objects))

	for _, object := range objects {
		service, ok := object.(*corev1.Service)
		if !ok {
			continue
		}
		services = append(services, toServiceInfo(service, now))
	}

	sortByNamespaceAndName(
		services,
		func(service ServiceInfo) string { return service.Namespace },
		func(service ServiceInfo) string { return service.Name },
	)

	return services
}

// toServiceInfo flattens a service the way `kubectl get services -A` presents it.
func toServiceInfo(service *corev1.Service, now time.Time) ServiceInfo {
	return ServiceInfo{
		Namespace:  service.Namespace,
		Name:       service.Name,
		Type:       string(service.Spec.Type),
		ClusterIP:  serviceClusterIP(service),
		ExternalIP: serviceExternalIP(service),
		Ports:      servicePorts(service.Spec.Ports),
		Age:        duration.HumanDuration(now.Sub(service.CreationTimestamp.Time)),
	}
}

// serviceClusterIP is kubectl's CLUSTER-IP column: the first cluster IP, which reads "None" for a
// headless service, and "<none>" for a service that has no cluster IP at all, such as an
// ExternalName one.
func serviceClusterIP(service *corev1.Service) string {
	if len(service.Spec.ClusterIPs) == 0 {
		return "<none>"
	}

	return service.Spec.ClusterIPs[0]
}

// serviceExternalIP is kubectl's EXTERNAL-IP column, placeholders included: a load balancer that
// has not been given an address yet reads "<pending>", while a kind that exposes nothing on its
// own reads "<none>" unless it was given explicit external IPs.
func serviceExternalIP(service *corev1.Service) string {
	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		addresses := loadBalancerAddresses(service.Status.LoadBalancer)
		explicit := strings.Join(service.Spec.ExternalIPs, ",")

		// kubectl lists what the load balancer reported first, then the explicit entries.
		switch {
		case addresses != "" && explicit != "":
			return addresses + "," + explicit
		case addresses != "":
			return addresses
		case explicit != "":
			return explicit
		default:
			return "<pending>"
		}
	case corev1.ServiceTypeExternalName:
		return service.Spec.ExternalName
	default:
		if len(service.Spec.ExternalIPs) == 0 {
			return "<none>"
		}

		return strings.Join(service.Spec.ExternalIPs, ",")
	}
}

// loadBalancerAddresses collects what a load balancer reported, deduplicated and sorted the way
// kubectl collects it. An ingress entry carries either an IP or a hostname, never both.
func loadBalancerAddresses(status corev1.LoadBalancerStatus) string {
	addresses := make([]string, 0, len(status.Ingress))

	for _, ingress := range status.Ingress {
		address := ingress.IP
		if address == "" {
			address = ingress.Hostname
		}
		if address == "" || slices.Contains(addresses, address) {
			continue
		}

		addresses = append(addresses, address)
	}

	if len(addresses) == 0 {
		return ""
	}

	sort.Strings(addresses)

	return strings.Join(addresses, ",")
}

// servicePorts is kubectl's PORT(S) column: "port/protocol", or "port:nodePort/protocol" when the
// service publishes a node port. A service without ports reads "<none>".
func servicePorts(ports []corev1.ServicePort) string {
	if len(ports) == 0 {
		return "<none>"
	}

	pieces := make([]string, 0, len(ports))
	for _, port := range ports {
		if port.NodePort > 0 {
			pieces = append(pieces, fmt.Sprintf("%d:%d/%s", port.Port, port.NodePort, port.Protocol))
			continue
		}
		pieces = append(pieces, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
	}

	return strings.Join(pieces, ",")
}
