package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestNetworkPolicyPodSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector metav1.LabelSelector
		want     string
	}{
		{
			// An empty selector selects every pod in the namespace, which kubectl prints the same
			// way as no selector at all.
			name: "an empty selector selects everything",
			want: "<none>",
		},
		{
			name: "one match label",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			want: "app=nginx",
		},
		{
			name: "match labels are sorted by key",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"tier": "web", "app": "nginx"},
			},
			want: "app=nginx,tier=web",
		},
		{
			name: "a match expression is rendered as a set membership",
			selector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"api", "web"}},
				},
			},
			want: "app in (api,web)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := &networkingv1.NetworkPolicy{Spec: networkingv1.NetworkPolicySpec{PodSelector: test.selector}}

			if got := toNetworkPolicyInfo(policy, time.Now()).PodSelector; got != test.want {
				t.Errorf("PodSelector = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToNetworkPolicyInfo(t *testing.T) {
	now := time.Now()

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "shop",
			Name:              "allow-web",
			CreationTimestamp: metav1.NewTime(now.Add(-26 * time.Hour)),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}

	got := toNetworkPolicyInfo(policy, now)

	want := NetworkPolicyInfo{
		Namespace:   "shop",
		Name:        "allow-web",
		PodSelector: "app=web",
		Age:         "26h",
	}
	if got != want {
		t.Errorf("toNetworkPolicyInfo() = %+v, want %+v", got, want)
	}
}

func TestNetworkPoliciesFromStoreSortsByNamespaceAndName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	entries := []*networkingv1.NetworkPolicy{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "data", Name: "cache"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"}},
	}
	for _, policy := range entries {
		if err := store.Add(policy); err != nil {
			t.Fatalf("add %s/%s: %v", policy.Namespace, policy.Name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add pod: %v", err)
	}

	got := networkPoliciesFromStore(store)

	want := []string{"data/cache", "shop/api", "shop/web"}
	if len(got) != len(want) {
		t.Fatalf("got %d network policies, want %d", len(got), len(want))
	}
	for i, key := range want {
		gotKey := got[i].Namespace + "/" + got[i].Name
		if gotKey != key {
			t.Errorf("network policy %d = %q, want %q", i, gotKey, key)
		}
	}
}

func TestNetworkPolicyYAML(t *testing.T) {
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "allow-web"},
	}

	t.Run("sets the type fields by hand and drops managedFields", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(policy)

		document, err := NetworkPolicyYAML(context.Background(), clientset, "shop", "allow-web")
		if err != nil {
			t.Fatalf("NetworkPolicyYAML() = %v, want nil", err)
		}

		for _, want := range []string{"kind: NetworkPolicy", "apiVersion: networking.k8s.io/v1", "name: allow-web"} {
			if !strings.Contains(document, want) {
				t.Errorf("document does not contain %q:\n%s", want, document)
			}
		}
		if strings.Contains(document, "managedFields") {
			t.Errorf("document still carries managedFields:\n%s", document)
		}
	})

	t.Run("names the network policy it could not read", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		_, err := NetworkPolicyYAML(context.Background(), clientset, "shop", "missing")
		if err == nil {
			t.Fatal("NetworkPolicyYAML() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Errorf("error %q does not name the network policy", err)
		}
	})
}
