package kube

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestToGatewayInfo(t *testing.T) {
	now := time.Now()

	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "web",
			Name:              "public",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
		},
		Spec: gatewayv1.GatewaySpec{GatewayClassName: "cilium"},
		Status: gatewayv1.GatewayStatus{
			Addresses: []gatewayv1.GatewayStatusAddress{
				{Value: "10.0.0.2"},
				{Value: "10.0.0.1"},
			},
			Conditions: []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionTrue}},
		},
	}

	got := toGatewayInfo(gateway, now)

	if got.Namespace != "web" || got.Name != "public" {
		t.Errorf("namespace/name = %q/%q, want web/public", got.Namespace, got.Name)
	}
	if got.Class != "cilium" {
		t.Errorf("class = %q, want cilium", got.Class)
	}
	if got.Address != "10.0.0.2,10.0.0.1" {
		t.Errorf("address = %q, want 10.0.0.2,10.0.0.1", got.Address)
	}
	if got.Programmed != "True" {
		t.Errorf("programmed = %q, want True", got.Programmed)
	}
	if got.Age != "5h" {
		t.Errorf("age = %q, want 5h", got.Age)
	}
}

func TestToGatewayInfoNotProgrammedYet(t *testing.T) {
	// A gateway the controller has not bound an address for yet reports both placeholders, the
	// same way it reads on the cluster before it is up.
	got := toGatewayInfo(&gatewayv1.Gateway{}, time.Now())

	if got.Address != "<none>" {
		t.Errorf("address without status = %q, want <none>", got.Address)
	}
	if got.Programmed != "<none>" {
		t.Errorf("programmed without status = %q, want <none>", got.Programmed)
	}
}

func TestGatewaysFromStoreSortsByNameAndNamespace(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, ref := range []struct{ namespace, name string }{
		{"default", "zebra"},
		{"kube-system", "alpha"},
		{"default", "alpha"},
	} {
		gateway := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: ref.namespace, Name: ref.name}}
		if err := store.Add(gateway); err != nil {
			t.Fatalf("add %s/%s: %v", ref.namespace, ref.name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add http route: %v", err)
	}

	got := gatewaysFromStore(store)

	want := []string{"default/alpha", "default/zebra", "kube-system/alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %d gateways, want %d", len(got), len(want))
	}
	for i, key := range want {
		if actual := got[i].Namespace + "/" + got[i].Name; actual != key {
			t.Errorf("gateway %d = %q, want %q", i, actual, key)
		}
	}
}
