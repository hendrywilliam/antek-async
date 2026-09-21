package kube

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestDecodeObject(t *testing.T) {
	tests := []struct {
		name     string
		document string
		wantKind string
		wantName string
		wantErr  string
	}{
		{
			name: "yaml deployment",
			document: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: web
`,
			wantKind: "Deployment",
			wantName: "nginx",
		},
		{
			name:     "json object",
			document: `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"app-config"}}`,
			wantKind: "ConfigMap",
			wantName: "app-config",
		},
		{
			name:     "empty document",
			document: "   \n\t\n",
			wantErr:  "Manifest is empty",
		},
		{
			name: "missing apiVersion",
			document: `kind: ConfigMap
metadata:
  name: app-config
`,
			wantErr: "apiVersion",
		},
		{
			name: "missing kind",
			document: `apiVersion: v1
metadata:
  name: app-config
`,
			wantErr: "kind",
		},
		{
			name: "missing name",
			document: `apiVersion: v1
kind: ConfigMap
metadata:
  namespace: web
`,
			wantErr: "metadata.name",
		},
		{
			name: "several documents",
			document: `apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second
`,
			wantErr: "one document",
		},
		{
			name: "trailing separator is not a second document",
			document: `apiVersion: v1
kind: ConfigMap
metadata:
  name: only
---
`,
			wantKind: "ConfigMap",
			wantName: "only",
		},
		{
			name:     "not a mapping",
			document: "- one\n- two\n",
			wantErr:  "cannot parse manifest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			object, err := decodeObject(test.document)

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("decodeObject() error = nil, want %q", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("decodeObject() error = %q, want it to contain %q", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("decodeObject() unexpected error = %v", err)
			}
			if object.GetKind() != test.wantKind {
				t.Errorf("kind = %q, want %q", object.GetKind(), test.wantKind)
			}
			if object.GetName() != test.wantName {
				t.Errorf("name = %q, want %q", object.GetName(), test.wantName)
			}
		})
	}
}

func TestPlacement(t *testing.T) {
	namespaced := &meta.RESTMapping{Scope: meta.RESTScopeNamespace}
	clusterScoped := &meta.RESTMapping{Scope: meta.RESTScopeRoot}

	tests := []struct {
		name           string
		mapping        *meta.RESTMapping
		namespace      string
		wantNamespace  string
		wantNamespaced bool
	}{
		{
			name:           "namespaced with an explicit namespace",
			mapping:        namespaced,
			namespace:      "web",
			wantNamespace:  "web",
			wantNamespaced: true,
		},
		{
			name:           "namespaced without a namespace falls back to default",
			mapping:        namespaced,
			wantNamespace:  "default",
			wantNamespaced: true,
		},
		{
			name:           "cluster scoped drops the namespace",
			mapping:        clusterScoped,
			namespace:      "web",
			wantNamespace:  "",
			wantNamespaced: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			namespace, namespaced := placement(test.mapping, test.namespace)

			if namespace != test.wantNamespace {
				t.Errorf("namespace = %q, want %q", namespace, test.wantNamespace)
			}
			if namespaced != test.wantNamespaced {
				t.Errorf("namespaced = %v, want %v", namespaced, test.wantNamespaced)
			}
		})
	}
}

// fakeDiscovery serves the two kinds the mapping test needs: a namespaced apps/v1 kind and the
// cluster-scoped core/v1 Node, so the cluster scope is exercised as well.
func fakeDiscovery() *fakediscovery.FakeDiscovery {
	// Resources is promoted from the embedded testing.Fake, so it is assigned after the literal
	// rather than inside it.
	discovery := &fakediscovery.FakeDiscovery{Fake: &ktesting.Fake{}}
	discovery.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "deployments",
					Kind:       "Deployment",
					Namespaced: true,
					Verbs:      metav1.Verbs{"create", "get", "list", "patch", "update", "watch"},
				},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:       "configmaps",
					Kind:       "ConfigMap",
					Namespaced: true,
					Verbs:      metav1.Verbs{"create", "get", "list", "patch", "update", "watch"},
				},
				{
					Name:       "nodes",
					Kind:       "Node",
					Namespaced: false,
					Verbs:      metav1.Verbs{"get", "list", "patch", "update", "watch"},
				},
			},
		},
	}

	return discovery
}

func TestResolveMapping(t *testing.T) {
	tests := []struct {
		name       string
		gvk        schema.GroupVersionKind
		wantGroup  string
		wantRes    string
		wantNamesp bool
		wantErr    string
	}{
		{
			name:       "namespaced group kind",
			gvk:        schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			wantGroup:  "apps",
			wantRes:    "deployments",
			wantNamesp: true,
		},
		{
			name:       "core group kind",
			gvk:        schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
			wantGroup:  "",
			wantRes:    "configmaps",
			wantNamesp: true,
		},
		{
			name:       "cluster scoped kind",
			gvk:        schema.GroupVersionKind{Version: "v1", Kind: "Node"},
			wantGroup:  "",
			wantRes:    "nodes",
			wantNamesp: false,
		},
		{
			name:    "unknown kind",
			gvk:     schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Widget"},
			wantErr: "unknown kind Widget",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapping, err := resolveMapping(fakeDiscovery(), test.gvk)

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveMapping() error = nil, want %q", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("resolveMapping() error = %q, want it to contain %q", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolveMapping() unexpected error = %v", err)
			}
			if mapping.Resource.Group != test.wantGroup {
				t.Errorf("resource group = %q, want %q", mapping.Resource.Group, test.wantGroup)
			}
			if mapping.Resource.Resource != test.wantRes {
				t.Errorf("resource = %q, want %q", mapping.Resource.Resource, test.wantRes)
			}
			namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
			if namespaced != test.wantNamesp {
				t.Errorf("namespaced = %v, want %v", namespaced, test.wantNamesp)
			}
		})
	}
}
