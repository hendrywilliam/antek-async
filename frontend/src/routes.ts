// Route metadata drives the sidebar and the header. Every menu is its own hash route; which
// cluster kind a page streams is the page's own business, so it is not listed here.
export type View =
	| "node"
	| "pod"
	| "deployment"
	| "statefulset"
	| "editor"
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
	editor: {
		view: "editor",
		path: "/editor",
		label: "Editor",
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
