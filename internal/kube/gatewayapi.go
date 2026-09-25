package kube

import (
	"errors"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// The four Gateway API kinds share the logic below, so it lives here instead of being copied
// into each of their files.

// gatewayAPIGroupVersion is the one version these kinds are read at, which is also the version
// their YAML views report.
const gatewayAPIGroupVersion = "gateway.networking.k8s.io/v1"

// conditionStatus reads one condition's status by type. The GatewayClass ACCEPTED and Gateway
// PROGRAMMED columns are exactly this, and a kind that has not been reconciled yet carries no
// such condition at all.
func conditionStatus(conditions []metav1.Condition, conditionType string) string {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return string(condition.Status)
		}
	}

	return "<none>"
}

// joinOrNone is the column convention for a list that kubectl would render as "<none>" when it
// is empty. The values keep the order the resource stores them in, because the CRDs' own
// printer columns join the list as stored rather than sorting it.
func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "<none>"
	}

	return strings.Join(values, ",")
}

// gatewayAddresses is the Gateway ADDRESS column: the values of status.addresses, which is what
// the CRD's own printer column reads.
func gatewayAddresses(status gatewayv1.GatewayStatus) string {
	addresses := make([]string, 0, len(status.Addresses))
	for _, address := range status.Addresses {
		if address.Value == "" {
			continue
		}
		addresses = append(addresses, address.Value)
	}

	return joinOrNone(addresses)
}

// routeHostnames is the HTTPRoute and GRPCRoute HOSTNAMES column.
func routeHostnames(hostnames []gatewayv1.Hostname) string {
	values := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		values = append(values, string(hostname))
	}

	return joinOrNone(values)
}

// routeParentRefs is the HTTPRoute and GRPCRoute PARENT REFS column: the gateways a route asks to
// attach to, read from spec.parentRefs. A reference without a namespace means the route's own
// namespace, so it is resolved here: that is what makes the value usable as a grouping key across
// namespaces, rather than one "my-gateway" bucket for every namespace at once. The list keeps its
// stored order, and a reference that names a listener section keeps it.
func routeParentRefs(parentRefs []gatewayv1.ParentReference, routeNamespace string) string {
	refs := make([]string, 0, len(parentRefs))

	for _, parent := range parentRefs {
		namespace := routeNamespace
		if parent.Namespace != nil && *parent.Namespace != "" {
			namespace = string(*parent.Namespace)
		}

		ref := string(parent.Name)
		if namespace != "" {
			ref = namespace + "/" + ref
		}
		if parent.SectionName != nil && *parent.SectionName != "" {
			ref += ":" + string(*parent.SectionName)
		}

		refs = append(refs, ref)
	}

	return joinOrNone(refs)
}

// gatewayAPIError names what is actually missing when the cluster does not serve the CRDs. The
// API server's own 404 ("the server could not find the requested resource") never says Gateway
// API, and a cluster without the CRDs is the common case rather than a fault in the request.
func gatewayAPIError(err error) error {
	if apierrors.IsNotFound(err) {
		return errors.New("the Gateway API CRDs are not installed")
	}

	return err
}
