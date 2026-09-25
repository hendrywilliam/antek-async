package kube

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestToHTTPRouteInfo(t *testing.T) {
	now := time.Now()

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "web",
			Name:              "api",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames:  []gatewayv1.Hostname{"www.example.com", "api.example.com"},
			ParentRefs: []gatewayv1.ParentReference{{Name: "public"}},
		},
	}

	got := toHTTPRouteInfo(route, now)

	if got.Namespace != "web" || got.Name != "api" {
		t.Errorf("namespace/name = %q/%q, want web/api", got.Namespace, got.Name)
	}
	if got.Hostnames != "www.example.com,api.example.com" {
		t.Errorf("hostnames = %q, want www.example.com,api.example.com", got.Hostnames)
	}
	if got.ParentRefs != "web/public" {
		t.Errorf("parent refs = %q, want web/public", got.ParentRefs)
	}
	if got.Age != "5h" {
		t.Errorf("age = %q, want 5h", got.Age)
	}
}

func TestToHTTPRouteInfoWithoutHostnames(t *testing.T) {
	// A route without hostnames matches every host, and one without parent references is not
	// attached to anything yet: both read as the placeholder.
	got := toHTTPRouteInfo(&gatewayv1.HTTPRoute{}, time.Now())

	if got.Hostnames != "<none>" {
		t.Errorf("hostnames = %q, want <none>", got.Hostnames)
	}
	if got.ParentRefs != "<none>" {
		t.Errorf("parent refs = %q, want <none>", got.ParentRefs)
	}
}

func TestHTTPRoutesFromStoreSortsByNameAndNamespace(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, ref := range []struct{ namespace, name string }{
		{"default", "zebra"},
		{"kube-system", "alpha"},
		{"default", "alpha"},
	} {
		route := &gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: ref.namespace, Name: ref.name}}
		if err := store.Add(route); err != nil {
			t.Fatalf("add %s/%s: %v", ref.namespace, ref.name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&gatewayv1.GRPCRoute{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add grpc route: %v", err)
	}

	got := httpRoutesFromStore(store)

	want := []string{"default/alpha", "default/zebra", "kube-system/alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %d http routes, want %d", len(got), len(want))
	}
	for i, key := range want {
		if actual := got[i].Namespace + "/" + got[i].Name; actual != key {
			t.Errorf("http route %d = %q, want %q", i, actual, key)
		}
	}
}
