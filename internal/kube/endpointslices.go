package kube

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	yaml "sigs.k8s.io/yaml"
)

// listColumnMax is the number of entries kubectl prints in a PORTS or ENDPOINTS column before it
// summarises the rest as "+ N more...".
const listColumnMax = 3

// EndpointSliceInfo is the flattened endpoint slice representation rendered by the table.
type EndpointSliceInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	AddressType string `json:"addressType"`
	Ports       string `json:"ports"`
	Endpoints   string `json:"endpoints"`
	Age         string `json:"age"`
}

// WatchEndpointSlices streams endpoint slice snapshots for one client, across all namespaces. It
// returns when ctx is done, or when the probe or the first cache sync fails.
func WatchEndpointSlices(
	ctx context.Context,
	clientset kubernetes.Interface,
	onSnapshot func([]EndpointSliceInfo),
) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"EndpointSlice",
		factory,
		factory.Discovery().V1().EndpointSlices().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.DiscoveryV1().EndpointSlices("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		endpointSlicesFromStore,
		onSnapshot,
	)
}

func endpointSlicesFromStore(store cache.Store) []EndpointSliceInfo {
	now := time.Now()
	objects := store.List()
	slices := make([]EndpointSliceInfo, 0, len(objects))

	for _, object := range objects {
		slice, ok := object.(*discoveryv1.EndpointSlice)
		if !ok {
			continue
		}
		slices = append(slices, toEndpointSliceInfo(slice, now))
	}

	sortByNamespaceAndName(
		slices,
		func(slice EndpointSliceInfo) string { return slice.Namespace },
		func(slice EndpointSliceInfo) string { return slice.Name },
	)

	return slices
}

// toEndpointSliceInfo flattens an endpoint slice the way `kubectl get endpointslices -A`
// presents it. ADDRESSTYPE is repeated per slice because one service is normally split across an
// IPv4 and an IPv6 slice, which is the whole reason to look at this kind.
func toEndpointSliceInfo(slice *discoveryv1.EndpointSlice, now time.Time) EndpointSliceInfo {
	return EndpointSliceInfo{
		Namespace:   slice.Namespace,
		Name:        slice.Name,
		AddressType: string(slice.AddressType),
		Ports:       endpointSlicePorts(slice.Ports),
		Endpoints:   endpointSliceEndpoints(slice.Endpoints),
		Age:         duration.HumanDuration(now.Sub(slice.CreationTimestamp.Time)),
	}
}

// endpointSlicePorts is kubectl's PORTS column: the port number, the port's name when it has no
// number, or "*" for a slice that declares the port by name only.
func endpointSlicePorts(ports []discoveryv1.EndpointPort) string {
	pieces := make([]string, 0, len(ports))

	for _, port := range ports {
		piece := "*"
		if port.Port != nil {
			piece = strconv.Itoa(int(*port.Port))
		} else if port.Name != nil {
			piece = *port.Name
		}
		pieces = append(pieces, piece)
	}

	return cappedList(pieces)
}

// endpointSliceEndpoints is kubectl's ENDPOINTS column: every address of every endpoint in
// stored order, because the API server renders those as-is rather than sorting them.
func endpointSliceEndpoints(endpoints []discoveryv1.Endpoint) string {
	addresses := make([]string, 0, len(endpoints))

	for _, endpoint := range endpoints {
		addresses = append(addresses, endpoint.Addresses...)
	}

	return cappedList(addresses)
}

// cappedList renders the list columns kubectl itself caps: the first three entries joined with
// commas, a "+ N more..." tail once there are more, and "<unset>" when there are none at all.
func cappedList(items []string) string {
	switch {
	case len(items) == 0:
		return "<unset>"
	case len(items) <= listColumnMax:
		return strings.Join(items, ",")
	default:
		return fmt.Sprintf("%s + %d more...", strings.Join(items[:listColumnMax], ","), len(items)-listColumnMax)
	}
}

// EndpointSliceYAML renders one endpoint slice the way `kubectl get endpointslice -o yaml` prints
// it. Like PodYAML the type fields are set by hand, because the typed client leaves them empty,
// and managedFields is dropped the way kubectl hides that bookkeeping by default.
func EndpointSliceYAML(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	slice, err := clientset.DiscoveryV1().EndpointSlices(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read EndpointSlice %s/%s: %w", namespace, name, err)
	}

	slice.APIVersion = "discovery.k8s.io/v1"
	slice.Kind = "EndpointSlice"
	slice.ManagedFields = nil

	document, err := yaml.Marshal(slice)
	if err != nil {
		return "", fmt.Errorf("cannot encode EndpointSlice %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}
