package kube

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestToStatefulSetInfo(t *testing.T) {
	now := time.Now()
	replicas := int32(3)

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "data",
			Name:              "postgres",
			CreationTimestamp: metav1.NewTime(now.Add(-3 * time.Hour)),
		},
		Spec: appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ReadyReplicas: 1,
		},
	}

	got := toStatefulSetInfo(statefulSet, now)

	if got.Namespace != "data" || got.Name != "postgres" {
		t.Errorf("namespace/name = %q/%q, want data/postgres", got.Namespace, got.Name)
	}
	if got.Ready != "1/3" {
		t.Errorf("ready = %q, want 1/3", got.Ready)
	}
	if got.Age != "3h" {
		t.Errorf("age = %q, want 3h", got.Age)
	}

	// A statefulset without spec.replicas reports a desired count of zero, like kubectl.
	if info := toStatefulSetInfo(&appsv1.StatefulSet{}, now); info.Ready != "0/0" {
		t.Errorf("ready without replicas = %q, want 0/0", info.Ready)
	}
}

func TestStatefulSetsFromStoreSortsByNameAndNamespace(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, ref := range []struct{ namespace, name string }{
		{"data", "zebra"},
		{"infra", "alpha"},
		{"data", "alpha"},
	} {
		statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: ref.namespace, Name: ref.name}}
		if err := store.Add(statefulSet); err != nil {
			t.Fatalf("add %s/%s: %v", ref.namespace, ref.name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add deployment: %v", err)
	}

	got := statefulSetsFromStore(store)

	want := []string{"data/alpha", "data/zebra", "infra/alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %d statefulsets, want %d", len(got), len(want))
	}
	for i, key := range want {
		if actual := got[i].Namespace + "/" + got[i].Name; actual != key {
			t.Errorf("statefulset %d = %q, want %q", i, actual, key)
		}
	}
}
