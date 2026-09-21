package kube

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestToDeploymentInfo(t *testing.T) {
	now := time.Now()
	replicas := int32(3)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "production",
			Name:              "api",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas:     2,
			UpdatedReplicas:   3,
			AvailableReplicas: 2,
		},
	}

	got := toDeploymentInfo(deployment, now)

	if got.Namespace != "production" || got.Name != "api" {
		t.Errorf("namespace/name = %q/%q, want production/api", got.Namespace, got.Name)
	}
	if got.Ready != "2/3" {
		t.Errorf("ready = %q, want 2/3", got.Ready)
	}
	if got.UpToDate != 3 {
		t.Errorf("upToDate = %d, want 3", got.UpToDate)
	}
	if got.Available != 2 {
		t.Errorf("available = %d, want 2", got.Available)
	}
	if got.Age != "5h" {
		t.Errorf("age = %q, want 5h", got.Age)
	}

	// A deployment without spec.replicas reports a desired count of zero, like kubectl.
	if info := toDeploymentInfo(&appsv1.Deployment{}, now); info.Ready != "0/0" {
		t.Errorf("ready without replicas = %q, want 0/0", info.Ready)
	}
}

func TestDeploymentsFromStoreSortsByNameAndNamespace(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, ref := range []struct{ namespace, name string }{
		{"default", "zebra"},
		{"kube-system", "alpha"},
		{"default", "alpha"},
	} {
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: ref.namespace, Name: ref.name}}
		if err := store.Add(deployment); err != nil {
			t.Fatalf("add %s/%s: %v", ref.namespace, ref.name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add statefulset: %v", err)
	}

	got := deploymentsFromStore(store)

	want := []string{"default/alpha", "default/zebra", "kube-system/alpha"}
	if len(got) != len(want) {
		t.Fatalf("got %d deployments, want %d", len(got), len(want))
	}
	for i, key := range want {
		if actual := got[i].Namespace + "/" + got[i].Name; actual != key {
			t.Errorf("deployment %d = %q, want %q", i, actual, key)
		}
	}
}
