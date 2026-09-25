package kube

import (
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestConditionStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		condition  string
		want       string
	}{
		{
			name:       "accepted is true",
			conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}},
			condition:  "Accepted",
			want:       "True",
		},
		{
			name:       "programmed is false",
			conditions: []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionFalse}},
			condition:  "Programmed",
			want:       "False",
		},
		{
			name:       "the controller has not decided yet",
			conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionUnknown}},
			condition:  "Accepted",
			want:       "Unknown",
		},
		{
			// Only the requested type is read, and the other conditions are not a substitute.
			name:       "the requested condition is absent",
			conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}},
			condition:  "Programmed",
			want:       "<none>",
		},
		{
			name:      "no conditions at all",
			condition: "Accepted",
			want:      "<none>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := conditionStatus(test.conditions, test.condition); got != test.want {
				t.Errorf("conditionStatus(%q) = %q, want %q", test.condition, got, test.want)
			}
		})
	}
}

func TestGatewayAddresses(t *testing.T) {
	tests := []struct {
		name   string
		status gatewayv1.GatewayStatus
		want   string
	}{
		{
			name: "no address bound yet",
			want: "<none>",
		},
		{
			name:   "one address",
			status: gatewayv1.GatewayStatus{Addresses: []gatewayv1.GatewayStatusAddress{{Value: "10.0.0.1"}}},
			want:   "10.0.0.1",
		},
		{
			// The CRD's own printer column joins the list as stored rather than sorting it.
			name: "several addresses keep their order",
			status: gatewayv1.GatewayStatus{Addresses: []gatewayv1.GatewayStatusAddress{
				{Value: "10.0.0.2"},
				{Value: "10.0.0.1"},
			}},
			want: "10.0.0.2,10.0.0.1",
		},
		{
			name:   "an entry without a value is skipped",
			status: gatewayv1.GatewayStatus{Addresses: []gatewayv1.GatewayStatusAddress{{Value: ""}}},
			want:   "<none>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gatewayAddresses(test.status); got != test.want {
				t.Errorf("gatewayAddresses() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRouteHostnames(t *testing.T) {
	tests := []struct {
		name      string
		hostnames []gatewayv1.Hostname
		want      string
	}{
		{
			// No hostnames means every host, which kubectl prints as the placeholder.
			name: "no hostnames",
			want: "<none>",
		},
		{
			name:      "one hostname",
			hostnames: []gatewayv1.Hostname{"api.example.com"},
			want:      "api.example.com",
		},
		{
			name:      "several hostnames keep their order",
			hostnames: []gatewayv1.Hostname{"www.example.com", "api.example.com"},
			want:      "www.example.com,api.example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := routeHostnames(test.hostnames); got != test.want {
				t.Errorf("routeHostnames() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGatewayAPIError(t *testing.T) {
	missing := apierrors.NewNotFound(
		schema.GroupResource{Group: "gateway.networking.k8s.io", Resource: "gateways"},
		"public",
	)

	got := gatewayAPIError(missing)
	if got == nil || !strings.Contains(got.Error(), "not installed") {
		t.Fatalf("gatewayAPIError(missing CRD) = %v, want a message naming the missing CRDs", got)
	}

	// Everything that is not a missing CRD is the API server's own problem and has to survive
	// untouched, so a permission failure still reads as a permission failure.
	forbidden := errors.New("forbidden")
	if got := gatewayAPIError(forbidden); got != forbidden {
		t.Errorf("gatewayAPIError(forbidden) = %v, want the original error", got)
	}

	if got := gatewayAPIError(nil); got != nil {
		t.Errorf("gatewayAPIError(nil) = %v, want nil", got)
	}
}

func TestRouteParentRefs(t *testing.T) {
	edge := gatewayv1.Namespace("edge")
	https := gatewayv1.SectionName("https")

	tests := []struct {
		name           string
		parentRefs     []gatewayv1.ParentReference
		routeNamespace string
		want           string
	}{
		{
			name:           "a route with no parent references",
			routeNamespace: "web",
			want:           "<none>",
		},
		{
			// A bare name means "in my own namespace", which is what makes the value a usable
			// grouping key across namespaces.
			name:           "a bare name resolves to the route's namespace",
			parentRefs:     []gatewayv1.ParentReference{{Name: "public"}},
			routeNamespace: "web",
			want:           "web/public",
		},
		{
			name:           "an explicit namespace wins",
			parentRefs:     []gatewayv1.ParentReference{{Name: "public", Namespace: &edge}},
			routeNamespace: "web",
			want:           "edge/public",
		},
		{
			name:           "a listener section is kept",
			parentRefs:     []gatewayv1.ParentReference{{Name: "public", SectionName: &https}},
			routeNamespace: "web",
			want:           "web/public:https",
		},
		{
			name: "several references keep their order",
			parentRefs: []gatewayv1.ParentReference{
				{Name: "second"},
				{Name: "first"},
			},
			routeNamespace: "web",
			want:           "web/second,web/first",
		},
		{
			name:       "a route without a namespace leaves the bare name",
			parentRefs: []gatewayv1.ParentReference{{Name: "public"}},
			want:       "public",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := routeParentRefs(test.parentRefs, test.routeNamespace); got != test.want {
				t.Errorf("routeParentRefs() = %q, want %q", got, test.want)
			}
		})
	}
}
