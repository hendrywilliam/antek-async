package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func TestToNamespaceInfo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		namespace *corev1.Namespace
		want      NamespaceInfo
	}{
		{
			name: "active namespace",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "default",
					CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Hour)),
				},
				Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
			},
			want: NamespaceInfo{Name: "default", Status: "Active", Age: "5h"},
		},
		{
			name: "terminating namespace",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "doomed",
					CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Minute)),
				},
				Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
			},
			want: NamespaceInfo{Name: "doomed", Status: "Terminating", Age: "2m"},
		},
		{
			// kubectl prints the phase verbatim, so a namespace whose controller never
			// reconciled keeps reading Active even while its deletion timestamp is set.
			name: "deleting namespace without a phase change",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "stuck",
					CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Second)),
					DeletionTimestamp: &metav1.Time{Time: now.Add(-5 * time.Second)},
					Finalizers:        []string{"kubernetes"},
				},
				Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
			},
			want: NamespaceInfo{Name: "stuck", Status: "Active", Age: "30s"},
		},
		{
			name: "namespace without a phase",
			namespace: &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
				Name:              "empty",
				CreationTimestamp: metav1.NewTime(now),
			}},
			want: NamespaceInfo{Name: "empty", Status: "", Age: "0s"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := toNamespaceInfo(test.namespace, now); got != test.want {
				t.Errorf("toNamespaceInfo() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestNamespacesFromStoreSortsByName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	for _, name := range []string{"kube-system", "default", "argocd"} {
		namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := store.Add(namespace); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add pod: %v", err)
	}

	got := namespacesFromStore(store)

	want := []string{"argocd", "default", "kube-system"}
	if len(got) != len(want) {
		t.Fatalf("got %d namespaces, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("namespace %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestDeleteNamespace(t *testing.T) {
	t.Run("removes the namespace", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "doomed"},
		})

		if err := DeleteNamespace(context.Background(), clientset, "doomed"); err != nil {
			t.Fatalf("DeleteNamespace() = %v, want nil", err)
		}

		namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(namespaces.Items) != 0 {
			t.Errorf("got %d namespaces left, want 0", len(namespaces.Items))
		}
	})

	t.Run("names the namespace it could not delete", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		err := DeleteNamespace(context.Background(), clientset, "missing")
		if err == nil {
			t.Fatal("DeleteNamespace() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Errorf("error %q does not name the namespace", err)
		}
	})
}
