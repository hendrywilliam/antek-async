package kube

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

// The endpoint port and endpoint name of an EndpointSlice are pointers, because the API server
// distinguishes an absent number from a zero one.
func int32Ptr(value int32) *int32 { return &value }

func stringPtr(value string) *string { return &value }

func TestEndpointSlicePorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []discoveryv1.EndpointPort
		want  string
	}{
		{
			name: "no ports",
			want: "<unset>",
		},
		{
			name: "a numbered port is printed as its number",
			ports: []discoveryv1.EndpointPort{
				{Name: stringPtr("http"), Port: int32Ptr(80)},
			},
			want: "80",
		},
		{
			name: "a port without a number falls back to its name",
			ports: []discoveryv1.EndpointPort{
				{Name: stringPtr("metrics")},
			},
			want: "metrics",
		},
		{
			name: "a port with neither name nor number is a wildcard",
			ports: []discoveryv1.EndpointPort{
				{},
			},
			want: "*",
		},
		{
			name: "several ports are joined",
			ports: []discoveryv1.EndpointPort{
				{Port: int32Ptr(80)},
				{Port: int32Ptr(443)},
				{Port: int32Ptr(8080)},
			},
			want: "80,443,8080",
		},
		{
			name: "more than three ports are summarised",
			ports: []discoveryv1.EndpointPort{
				{Port: int32Ptr(80)},
				{Port: int32Ptr(443)},
				{Port: int32Ptr(8080)},
				{Port: int32Ptr(9090)},
				{Port: int32Ptr(9091)},
			},
			want: "80,443,8080 + 2 more...",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := endpointSlicePorts(test.ports); got != test.want {
				t.Errorf("endpointSlicePorts() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEndpointSliceEndpoints(t *testing.T) {
	tests := []struct {
		name      string
		endpoints []discoveryv1.Endpoint
		want      string
	}{
		{
			name: "no endpoints",
			want: "<unset>",
		},
		{
			name: "every address of every endpoint is kept in stored order",
			endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.42.0.7", "10.42.0.8"}},
				{Addresses: []string{"10.42.1.9"}},
			},
			want: "10.42.0.7,10.42.0.8,10.42.1.9",
		},
		{
			name: "more than three addresses are summarised",
			endpoints: []discoveryv1.Endpoint{
				{Addresses: []string{"10.42.0.7", "10.42.0.8"}},
				{Addresses: []string{"10.42.1.9", "10.42.1.10", "10.42.1.11"}},
			},
			want: "10.42.0.7,10.42.0.8,10.42.1.9 + 2 more...",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := endpointSliceEndpoints(test.endpoints); got != test.want {
				t.Errorf("endpointSliceEndpoints() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToEndpointSliceInfo(t *testing.T) {
	now := time.Now()

	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "shop",
			Name:              "web-abc",
			CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports: []discoveryv1.EndpointPort{
			{Name: stringPtr("http"), Port: int32Ptr(8080)},
		},
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.42.0.7"}},
		},
	}

	got := toEndpointSliceInfo(slice, now)

	want := EndpointSliceInfo{
		Namespace:   "shop",
		Name:        "web-abc",
		AddressType: "IPv4",
		Ports:       "8080",
		Endpoints:   "10.42.0.7",
		Age:         "2h",
	}
	if got != want {
		t.Errorf("toEndpointSliceInfo() = %+v, want %+v", got, want)
	}
}

func TestEndpointSlicesFromStoreSortsByNamespaceAndName(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	entries := []*discoveryv1.EndpointSlice{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web-b"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "data", Name: "cache"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web-a"}},
	}
	for _, slice := range entries {
		if err := store.Add(slice); err != nil {
			t.Fatalf("add %s/%s: %v", slice.Namespace, slice.Name, err)
		}
	}

	// A store can hold other types too, and those must be skipped.
	if err := store.Add(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ignored"}}); err != nil {
		t.Fatalf("add pod: %v", err)
	}

	got := endpointSlicesFromStore(store)

	want := []string{"data/cache", "shop/web-a", "shop/web-b"}
	if len(got) != len(want) {
		t.Fatalf("got %d endpoint slices, want %d", len(got), len(want))
	}
	for i, key := range want {
		gotKey := got[i].Namespace + "/" + got[i].Name
		if gotKey != key {
			t.Errorf("endpoint slice %d = %q, want %q", i, gotKey, key)
		}
	}
}

func TestEndpointSliceYAML(t *testing.T) {
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Namespace: "shop", Name: "web-abc"},
		AddressType: discoveryv1.AddressTypeIPv4,
	}

	t.Run("sets the type fields by hand and drops managedFields", func(t *testing.T) {
		clientset := fake.NewSimpleClientset(slice)

		document, err := EndpointSliceYAML(context.Background(), clientset, "shop", "web-abc")
		if err != nil {
			t.Fatalf("EndpointSliceYAML() = %v, want nil", err)
		}

		for _, want := range []string{"kind: EndpointSlice", "apiVersion: discovery.k8s.io/v1", "name: web-abc"} {
			if !strings.Contains(document, want) {
				t.Errorf("document does not contain %q:\n%s", want, document)
			}
		}
		if strings.Contains(document, "managedFields") {
			t.Errorf("document still carries managedFields:\n%s", document)
		}
	})

	t.Run("names the endpoint slice it could not read", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()

		_, err := EndpointSliceYAML(context.Background(), clientset, "shop", "missing")
		if err == nil {
			t.Fatal("EndpointSliceYAML() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "missing") {
			t.Errorf("error %q does not name the endpoint slice", err)
		}
	})
}
