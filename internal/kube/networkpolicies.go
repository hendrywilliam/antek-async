package kube

import (
	"context"
	"fmt"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	yaml "sigs.k8s.io/yaml"
)

// NetworkPolicyInfo is the flattened network policy representation rendered by the table.
type NetworkPolicyInfo struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	PodSelector string `json:"podSelector"`
	Age         string `json:"age"`
}

// WatchNetworkPolicies streams network policy snapshots for one client, across all namespaces. It
// returns when ctx is done, or when the probe or the first cache sync fails.
func WatchNetworkPolicies(
	ctx context.Context,
	clientset kubernetes.Interface,
	onSnapshot func([]NetworkPolicyInfo),
) error {
	factory := informers.NewSharedInformerFactory(clientset, 0)

	return watchInformer(
		ctx,
		"NetworkPolicy",
		factory,
		factory.Networking().V1().NetworkPolicies().Informer(),
		func(probeCtx context.Context) error {
			_, err := clientset.NetworkingV1().NetworkPolicies("").List(probeCtx, metav1.ListOptions{Limit: 1})
			return err
		},
		networkPoliciesFromStore,
		onSnapshot,
	)
}

func networkPoliciesFromStore(store cache.Store) []NetworkPolicyInfo {
	now := time.Now()
	objects := store.List()
	policies := make([]NetworkPolicyInfo, 0, len(objects))

	for _, object := range objects {
		policy, ok := object.(*networkingv1.NetworkPolicy)
		if !ok {
			continue
		}
		policies = append(policies, toNetworkPolicyInfo(policy, now))
	}

	sortByNamespaceAndName(
		policies,
		func(policy NetworkPolicyInfo) string { return policy.Namespace },
		func(policy NetworkPolicyInfo) string { return policy.Name },
	)

	return policies
}

// toNetworkPolicyInfo flattens a network policy the way `kubectl get networkpolicies -A` presents
// it: POD-SELECTOR is the spec's own selector, rendered by the same helper kubectl uses, so a
// policy that selects every pod in its namespace reads "<none>" rather than an empty string.
func toNetworkPolicyInfo(policy *networkingv1.NetworkPolicy, now time.Time) NetworkPolicyInfo {
	return NetworkPolicyInfo{
		Namespace:   policy.Namespace,
		Name:        policy.Name,
		PodSelector: metav1.FormatLabelSelector(&policy.Spec.PodSelector),
		Age:         duration.HumanDuration(now.Sub(policy.CreationTimestamp.Time)),
	}
}

// NetworkPolicyYAML renders one network policy the way `kubectl get networkpolicy -o yaml`
// prints it. Like PodYAML the type fields are set by hand, because the typed client leaves them
// empty, and managedFields is dropped the way kubectl hides that bookkeeping by default.
func NetworkPolicyYAML(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	policy, err := clientset.NetworkingV1().NetworkPolicies(namespace).Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("cannot read NetworkPolicy %s/%s: %w", namespace, name, err)
	}

	policy.APIVersion = "networking.k8s.io/v1"
	policy.Kind = "NetworkPolicy"
	policy.ManagedFields = nil

	document, err := yaml.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("cannot encode NetworkPolicy %s/%s: %w", namespace, name, err)
	}

	return string(document), nil
}
