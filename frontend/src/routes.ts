// Route metadata drives the sidebar and the header. Every menu is its own hash route; which
// cluster kind a page streams is the page's own business, so it is not listed here.
export type View =
	| "node"
	| "namespace"
	| "pod"
	| "deployment"
	| "statefulset"
	| "service"
	| "gatewayClass"
	| "gateway"
	| "httpRoute"
	| "grpcRoute"
	| "manifest"
	| "settings";

export type RouteDef = {
	view: View;
	path: string;
	label: string;
	subtitle: string;
};

export const ROUTES: Record<View, RouteDef> = {
	pod: {
		view: "pod",
		path: "/pods",
		label: "Pods",
		subtitle: "All namespaces",
	},
	deployment: {
		view: "deployment",
		path: "/deployments",
		label: "Deployments",
		subtitle: "All namespaces",
	},
	statefulset: {
		view: "statefulset",
		path: "/statefulsets",
		label: "StatefulSets",
		subtitle: "All namespaces",
	},
	node: {
		view: "node",
		path: "/nodes",
		label: "Nodes",
		subtitle: "Cluster-wide",
	},
	namespace: {
		view: "namespace",
		path: "/namespaces",
		label: "Namespaces",
		subtitle: "Cluster-wide",
	},
	service: {
		view: "service",
		path: "/services",
		label: "Services",
		subtitle: "All namespaces",
	},
	// The Gateway API kinds are CRDs, so they are separate routes under the same Networking
	// menu rather than a resource this app compiles in.
	gatewayClass: {
		view: "gatewayClass",
		path: "/gateway-classes",
		label: "GatewayClass",
		subtitle: "Cluster-wide",
	},
	gateway: {
		view: "gateway",
		path: "/gateways",
		label: "Gateway",
		subtitle: "All namespaces",
	},
	httpRoute: {
		view: "httpRoute",
		path: "/http-routes",
		label: "HTTPRoute",
		subtitle: "All namespaces",
	},
	grpcRoute: {
		view: "grpcRoute",
		path: "/grpc-routes",
		label: "GRPCRoute",
		subtitle: "All namespaces",
	},
	manifest: {
		view: "manifest",
		path: "/manifest",
		label: "Manifest YAML",
		subtitle: "Apply YAML to the cluster",
	},
	settings: {
		view: "settings",
		path: "/settings",
		label: "Settings",
		subtitle: "Kubeconfig and connection",
	},
};

// Pods are what the backend already watches on startup, so they are the route the shell lands on.
export const DEFAULT_PATH = ROUTES.pod.path;

const BY_PATH = new Map(
	Object.values(ROUTES).map((route) => [route.path, route]),
);

export function routeForPath(pathname: string): RouteDef | null {
	return BY_PATH.get(pathname) ?? null;
}
