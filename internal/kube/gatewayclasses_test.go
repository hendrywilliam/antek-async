package kube

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestToGatewayClassInfo(t *testing.T) {
	now := time.Now()

	class := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "cilium",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
		},
		Spec: gatewayv1.GatewayClassSpec{ControllerName: "io.cilium/gateway-controller"},
		Status: gatewayv1.GatewayClassStatus{
			Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}},
		},
	}

	got := toGatewayClassInfo(class, now)

	if got.Name != "cilium" {
		t.Errorf("name = %q, want cilium", got.Name)
	}
	if got.Controller != "io.cilium/gateway-controller" {
		t.Errorf("controller = %q, want io.cilium/gateway-controller", got.Controller)
	}
	if got.Accepted != "True" {
		t.Errorf("accepted = %q, want True", got.Accepted)
	}
	if got.Age != "5h" {
		t.Errorf("age = %q, want 5h", got.Age)
	}
}

func TestToGatewayClassInfoWithoutStatus(t *testing.T) {
	// A class whose controller has not written status yet has no Accepted condition, which reads
	// as the placeholder rather than an empty cell.
	got := toGatewayClassInfo(&gatewayv1.GatewayClass{}, time.Now())

	if got.Accepted != "<none>" {
		t.Errorf("accepted without status = %q, want <none>", got.Accepted)
	}
}

func TestGatewayClassesFromStoreSortsByName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, name := range []string{"zebra", "alpha", "middle"} {
		class := &gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := store.Add(class); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add gateway: %v", err)
	}

	got := gatewayClassesFromStore(store)

	want := []string{"alpha", "middle", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("got %d gateway classes, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("gateway class %d = %q, want %q", i, got[i].Name, name)
		}
	}
}
