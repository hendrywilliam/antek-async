import {
	Fragment,
	type ComponentType,
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import {
	Box,
	Boxes,
	Database,
	Layers,
	LoaderCircle,
	MoreHorizontal,
	RotateCw,
	Server,
	Settings,
	X,
} from "lucide-react";
import { cn } from "cn";
import {
	GetPodYAML,
	GetState,
	PickKubeconfig,
	ResetKubeconfig,
	SelectResource,
} from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime";
import { kube, main } from "../wailsjs/go/models";
import { basicSetup, EditorView } from "codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { Button } from "@/components/ui/button";
import {
	Drawer,
	DrawerContent,
	DrawerDescription,
	DrawerHeader,
	DrawerTitle,
} from "@/components/ui/drawer";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import {
	Sidebar,
	SidebarContent,
	SidebarFooter,
	SidebarGroup,
	SidebarGroupContent,
	SidebarGroupLabel,
	SidebarHeader,
	SidebarInset,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarProvider,
	SidebarTrigger,
} from "@/components/ui/sidebar";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "@/components/ui/table";

const STATE_UPDATE_EVENT = "state:update";

// Radix Select treats an empty string as "no value", so the unfiltered choices need
// their own sentinel values.
const ALL_NAMESPACES = "__all__";
const NO_GROUPING = "__none__";

const OK_STATUSES = new Set(["Running", "Succeeded", "Completed"]);
const WARN_STATUSES = new Set([
	"Pending",
	"ContainerCreating",
	"PodInitializing",
	"NotReady",
	"Terminating",
]);

const TONE_CLASSES = {
	ok: "text-emerald-400",
	warn: "text-amber-400",
	bad: "text-red-400",
} as const;

const SOURCE_LABELS: Record<string, string> = {
	manual: "picked manually",
	KUBECONFIG: "env KUBECONFIG",
	home: "~/.kube/config",
	project: "<cwd>/.kube/config",
	none: "not found",
};

type View = "node" | "pod" | "deployment" | "statefulset" | "settings";

type Column<T> = {
	header: string;
	align?: "right";
	className?: string;
	render: (item: T) => ReactNode;
};

type GroupOption<T> = {
	value: string;
	label: string;
	key: (item: T) => string;
};

type Accessors<T> = {
	name: (item: T) => string;
	key: (item: T) => string;
	// Cluster-scoped kinds such as nodes have no namespace, which also hides the namespace
	// filter and namespace grouping for them.
	namespace?: (item: T) => string;
};

const VIEW_LABELS: Record<View, string> = {
	node: "Nodes",
	pod: "Pods",
	deployment: "Deployments",
	statefulset: "StatefulSets",
	settings: "Settings",
};

// Only the open menu is fetched, so opening a view maps to exactly one resource. These values
// must match the kube.Resource constants on the Go side.
const RESOURCE_OF: Record<View, string> = {
	node: "nodes",
	pod: "pods",
	deployment: "deployments",
	statefulset: "statefulSets",
	settings: "",
};

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_NODES: kube.NodeInfo[] = [];
const NO_PODS: kube.PodInfo[] = [];
const NO_DEPLOYMENTS: kube.DeploymentInfo[] = [];
const NO_STATEFULSETS: kube.StatefulSetInfo[] = [];

type MenuItem = {
	view: View;
	label: string;
	Icon: ComponentType<{ className?: string }>;
};

// Nodes are cluster scoped and get their own group above the namespaced workloads.
const MENU_GROUPS: { label: string; items: MenuItem[] }[] = [
	{
		label: "Cluster",
		items: [{ view: "node", label: "Nodes", Icon: Server }],
	},
	{
		label: "Workloads",
		items: [
			{ view: "pod", label: "Pods", Icon: Box },
			{ view: "deployment", label: "Deployments", Icon: Layers },
			{ view: "statefulset", label: "StatefulSets", Icon: Database },
		],
	},
];

const POD_ACCESSORS: Accessors<kube.PodInfo> = {
	namespace: (pod) => pod.namespace,
	name: (pod) => pod.name,
	key: (pod) => `${pod.namespace}/${pod.name}`,
};

const DEPLOYMENT_ACCESSORS: Accessors<kube.DeploymentInfo> = {
	namespace: (deployment) => deployment.namespace,
	name: (deployment) => deployment.name,
	key: (deployment) => `${deployment.namespace}/${deployment.name}`,
};

const STATEFULSET_ACCESSORS: Accessors<kube.StatefulSetInfo> = {
	namespace: (statefulSet) => statefulSet.namespace,
	name: (statefulSet) => statefulSet.name,
	key: (statefulSet) => `${statefulSet.namespace}/${statefulSet.name}`,
};

const NODE_ACCESSORS: Accessors<kube.NodeInfo> = {
	name: (node) => node.name,
	key: (node) => node.name,
};

const NODE_GROUPS: GroupOption<kube.NodeInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "status", label: "Status", key: (node) => node.status },
];

function namespaceGroups<T>(
	namespaceOf: (item: T) => string,
): GroupOption<T>[] {
	return [
		{ value: NO_GROUPING, label: "No grouping", key: () => "" },
		{ value: "namespace", label: "Namespace", key: namespaceOf },
	];
}

const POD_GROUPS: GroupOption<kube.PodInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (pod) => pod.namespace },
	{ value: "node", label: "Node", key: (pod) => pod.node },
	{ value: "status", label: "Status", key: (pod) => pod.status },
];

const DEPLOYMENT_GROUPS = namespaceGroups<kube.DeploymentInfo>(
	(deployment) => deployment.namespace,
);
const STATEFULSET_GROUPS = namespaceGroups<kube.StatefulSetInfo>(
	(statefulSet) => statefulSet.namespace,
);

const POD_COLUMNS: Column<kube.PodInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (pod) => pod.namespace,
	},
	{ header: "Name", className: "font-medium", render: (pod) => pod.name },
	{ header: "Ready", align: "right", render: (pod) => pod.ready },
	{ header: "Status", render: (pod) => <StatusLabel status={pod.status} /> },
	{ header: "Restarts", align: "right", render: (pod) => pod.restarts },
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (pod) => pod.age,
	},
	{ header: "IP", className: "text-muted-foreground", render: (pod) => pod.ip },
	{
		header: "Node",
		className: "text-muted-foreground",
		render: (pod) => pod.node,
	},
];

const DEPLOYMENT_COLUMNS: Column<kube.DeploymentInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (item) => item.namespace,
	},
	{ header: "Name", className: "font-medium", render: (item) => item.name },
	{ header: "Ready", align: "right", render: (item) => item.ready },
	{ header: "Up-to-date", align: "right", render: (item) => item.upToDate },
	{ header: "Available", align: "right", render: (item) => item.available },
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (item) => item.age,
	},
];

const STATEFULSET_COLUMNS: Column<kube.StatefulSetInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (item) => item.namespace,
	},
	{ header: "Name", className: "font-medium", render: (item) => item.name },
	{ header: "Ready", align: "right", render: (item) => item.ready },
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (item) => item.age,
	},
];

const NODE_COLUMNS: Column<kube.NodeInfo>[] = [
	{ header: "Name", className: "font-medium", render: (node) => node.name },
	{
		header: "Status",
		render: (node) => <NodeStatusLabel status={node.status} />,
	},
	{
		header: "Roles",
		className: "text-muted-foreground",
		render: (node) => node.roles,
	},
	{
		header: "Version",
		className: "text-muted-foreground",
		render: (node) => node.version,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (node) => node.age,
	},
];

function sourceLabel(source?: string): string {
	if (!source) {
		return "-";
	}
	return SOURCE_LABELS[source] ?? source;
}

function statusTone(status: string): "ok" | "warn" | "bad" {
	if (OK_STATUSES.has(status)) {
		return "ok";
	}
	if (WARN_STATUSES.has(status) || status.startsWith("Init:")) {
		return "warn";
	}
	return "bad";
}

function formatUpdatedAt(value?: string): string {
	if (!value) {
		return "-";
	}
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? "-" : date.toLocaleTimeString();
}

// Status is plain coloured text rather than a badge: green while healthy, amber while the
// pod is still coming up, and red for failures such as CrashLoopBackOff, ImagePullBackOff,
// Error, Failed or Evicted. Nothing else in the UI uses colour.
function StatusLabel({ status }: { status: string }) {
	return <span className={TONE_CLASSES[statusTone(status)]}>{status}</span>;
}

// Nodes reuse the coloured text, but a node that is NotReady is a real failure rather than a
// workload still coming up, so the mapping is its own.
function NodeStatusLabel({ status }: { status: string }) {
	const tone =
		status === "Ready" ? "ok" : status === "NotReady" ? "bad" : "warn";

	return <span className={TONE_CLASSES[tone]}>{status}</span>;
}

// ResourceTable renders one resource kind with its own filter, namespace selector and
// Group By menu. Filtering and grouping stay on the client because the snapshot is already
// in memory, so they never trigger extra cluster requests.
function ResourceTable<T>({
	rows,
	columns,
	groups: groupOptions,
	accessors,
	noun,
	loaded,
	loading,
	error,
	onRetry,
	rowActions,
}: {
	rows: T[];
	columns: Column<T>[];
	groups: GroupOption<T>[];
	accessors: Accessors<T>;
	noun: string;
	loaded: boolean;
	loading: boolean;
	error: string;
	onRetry: () => void;
	rowActions?: (item: T) => ReactNode;
}) {
	const [namespace, setNamespace] = useState(ALL_NAMESPACES);
	const [query, setQuery] = useState("");
	const [groupBy, setGroupBy] = useState(NO_GROUPING);

	const namespaces = useMemo(
		() =>
			accessors.namespace
				? [...new Set(rows.map(accessors.namespace))].sort()
				: [],
		[rows, accessors],
	);

	// A namespace can vanish when the kubeconfig changes or its workloads are deleted, so
	// fall back to "all" instead of leaving the select on a value that no longer exists.
	const activeNamespace = namespaces.includes(namespace)
		? namespace
		: ALL_NAMESPACES;

	const visible = useMemo(() => {
		const needle = query.trim().toLowerCase();
		const namespaceOf = accessors.namespace;

		return rows.filter((row) => {
			if (
				namespaceOf &&
				activeNamespace !== ALL_NAMESPACES &&
				namespaceOf(row) !== activeNamespace
			) {
				return false;
			}
			return (
				needle === "" || accessors.name(row).toLowerCase().includes(needle)
			);
		});
	}, [rows, activeNamespace, query, accessors]);

	const groups = useMemo(() => {
		const option = groupOptions.find(
			(candidate) => candidate.value === groupBy,
		);
		if (!option) {
			return [{ key: "", rows: visible }];
		}

		const byKey = new Map<string, T[]>();
		for (const row of visible) {
			const key = option.key(row);
			const bucket = byKey.get(key);
			if (bucket) {
				bucket.push(row);
			} else {
				byKey.set(key, [row]);
			}
		}

		// Rows keep the namespace/name ordering the backend already applied.
		return [...byKey.entries()]
			.sort(([left], [right]) => left.localeCompare(right))
			.map(([key, grouped]) => ({ key, rows: grouped }));
	}, [visible, groupBy, groupOptions]);

	const filtering =
		activeNamespace !== ALL_NAMESPACES ||
		query.trim() !== "" ||
		groupBy !== NO_GROUPING;

	const resetFilters = useCallback(() => {
		setNamespace(ALL_NAMESPACES);
		setQuery("");
		setGroupBy(NO_GROUPING);
	}, []);

	// Until the first fetch finishes there is nothing to filter, so show the spinner or the
	// failure instead of an empty table.
	if (!loaded) {
		return (
			<div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
				<div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border">
					{error !== "" ? (
						<div className="flex max-w-xl flex-col items-center gap-3 px-6 text-center">
							<p className="text-red-400">{error}</p>
							<Button onClick={onRetry} size="sm" variant="outline">
								Retry
							</Button>
						</div>
					) : loading ? (
						<p className="flex items-center gap-2 text-muted-foreground">
							<LoaderCircle className="size-4 animate-spin" />
							Loading {noun}…
						</p>
					) : (
						<p className="text-muted-foreground">Waiting…</p>
					)}
				</div>
			</div>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
			<div className="flex flex-wrap items-center gap-2">
				<Input
					className="w-64"
					onChange={(event) => setQuery(event.target.value)}
					placeholder={`Filter ${noun} name`}
					value={query}
				/>

				{accessors.namespace && (
					<Select onValueChange={setNamespace} value={activeNamespace}>
						<SelectTrigger className="w-52">
							<SelectValue placeholder="All namespaces" />
						</SelectTrigger>
						<SelectContent>
							<SelectItem value={ALL_NAMESPACES}>All namespaces</SelectItem>
							{namespaces.map((item) => (
								<SelectItem key={item} value={item}>
									{item}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
				)}

				<Select onValueChange={setGroupBy} value={groupBy}>
					<SelectTrigger className="w-44">
						<SelectValue placeholder="Group By" />
					</SelectTrigger>
					<SelectContent>
						{groupOptions.map((option) => (
							<SelectItem key={option.value} value={option.value}>
								{option.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>

				{filtering && (
					<Button onClick={resetFilters} size="sm" variant="ghost">
						Reset filters
					</Button>
				)}
			</div>

			<div className="min-h-0 flex-1 overflow-hidden rounded-lg border">
				<Table>
					<TableHeader className="sticky top-0 z-10 bg-background">
						<TableRow>
							{columns.map((column) => (
								<TableHead
									key={column.header}
									className={
										column.align === "right" ? "text-right" : undefined
									}
								>
									{column.header}
								</TableHead>
							))}
							{rowActions && <TableHead className="w-10" />}
						</TableRow>
					</TableHeader>
					<TableBody>
						{groups.map((group) => (
							<Fragment key={group.key || NO_GROUPING}>
								{groupBy !== NO_GROUPING && (
									<TableRow className="bg-muted/40 hover:bg-muted/40">
										<TableCell
											className="font-medium"
											colSpan={columns.length + (rowActions ? 1 : 0)}
										>
											{group.key}
											<span className="ml-2 text-muted-foreground">
												{group.rows.length}
											</span>
										</TableCell>
									</TableRow>
								)}
								{group.rows.map((row) => (
									<TableRow key={accessors.key(row)}>
										{columns.map((column) => (
											<TableCell
												key={column.header}
												className={cn(
													column.align === "right" && "text-right tabular-nums",
													column.className,
												)}
											>
												{column.render(row)}
											</TableCell>
										))}
										{rowActions && <TableCell>{rowActions(row)}</TableCell>}
									</TableRow>
								))}
							</Fragment>
						))}
					</TableBody>
				</Table>

				{visible.length === 0 && (
					<p className="py-10 text-center text-muted-foreground">
						{rows.length === 0
							? `No ${noun} in this cluster`
							: `No ${noun} matches the filter`}
					</p>
				)}
			</div>
		</div>
	);
}

function App() {
	const [state, setState] = useState<main.AppState | null>(null);
	const [fatal, setFatal] = useState("");
	const [busy, setBusy] = useState(false);
	const [view, setView] = useState<View>("pod");

	useEffect(() => {
		GetState()
			.then(setState)
			.catch((err) => setFatal(String(err)));

		// EventsOn returns an unsubscribe function. React StrictMode mounts effects
		// twice in development, so a leaked listener would duplicate every update.
		const off = EventsOn(STATE_UPDATE_EVENT, (next: main.AppState) =>
			setState(next),
		);

		return () => off();
	}, []);

	const run = useCallback(async (action: () => Promise<main.AppState>) => {
		setBusy(true);
		try {
			setState(await action());
			setFatal("");
		} catch (err) {
			setFatal(String(err));
		} finally {
			setBusy(false);
		}
	}, []);

	const config = state?.config;
	const nodes = state?.nodes;
	const pods = state?.pods;
	const deployments = state?.deployments;
	const statefulSets = state?.statefulSets;
	const error = fatal || state?.error || "";
	const configMissing = config != null && config.path === "";
	const activeResource = RESOURCE_OF[view];

	// Opening a view is what triggers the fetch, and Settings fetches nothing. Calling this for
	// the already open resource reconnects it, so the same path serves as the reload button.
	const openView = useCallback(
		(next: View) => {
			setView(next);
			if (RESOURCE_OF[next] !== "") {
				run(() => SelectResource(RESOURCE_OF[next]));
			}
		},
		[run],
	);

	const reloadActive = useCallback(() => {
		if (activeResource !== "") {
			run(() => SelectResource(activeResource));
		}
	}, [activeResource, run]);

	// The YAML drawer is independent of the active watch: it fetches one pod on demand, so it
	// works from any menu.
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);
	const [yamlDocument, setYamlDocument] = useState("");
	const [yamlError, setYamlError] = useState("");
	const [yamlLoading, setYamlLoading] = useState(false);

	const viewYAML = useCallback((namespace: string, name: string) => {
		setYamlTarget({ namespace, name });
		setYamlDocument("");
		setYamlError("");
		setYamlLoading(true);

		GetPodYAML(namespace, name)
			.then(setYamlDocument)
			.catch((err) => setYamlError(String(err)))
			.finally(() => setYamlLoading(false));
	}, []);

	const podRowActions = useCallback(
		(pod: kube.PodInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${pod.name}`}
						size="icon-sm"
						variant="ghost"
					>
						<MoreHorizontal />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem onSelect={() => viewYAML(pod.namespace, pod.name)}>
						View YAML
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		),
		[viewYAML],
	);

	return (
		<SidebarProvider className="h-svh overflow-hidden">
			<Sidebar>
				<SidebarHeader>
					<div className="flex items-center gap-2 px-2 py-1">
						<Boxes className="size-5" />
						<div className="grid leading-tight">
							<span className="font-semibold">antek-async</span>
							<span className="text-muted-foreground">Kubernetes monitor</span>
						</div>
					</div>
				</SidebarHeader>
				<SidebarContent>
					{MENU_GROUPS.map((group) => (
						<SidebarGroup key={group.label}>
							<SidebarGroupLabel>{group.label}</SidebarGroupLabel>
							<SidebarGroupContent>
								<SidebarMenu>
									{group.items.map((item) => (
										<SidebarMenuItem key={item.view}>
											<SidebarMenuButton
												isActive={view === item.view}
												onClick={() => openView(item.view)}
												tooltip={item.label}
											>
												<item.Icon />
												<span>{item.label}</span>
											</SidebarMenuButton>
										</SidebarMenuItem>
									))}
								</SidebarMenu>
							</SidebarGroupContent>
						</SidebarGroup>
					))}
				</SidebarContent>
				<SidebarFooter>
					<SidebarMenu>
						<SidebarMenuItem>
							<SidebarMenuButton
								isActive={view === "settings"}
								onClick={() => openView("settings")}
								tooltip="Settings"
							>
								<Settings />
								<span>Settings</span>
							</SidebarMenuButton>
						</SidebarMenuItem>
					</SidebarMenu>
				</SidebarFooter>
			</Sidebar>

			<SidebarInset className="flex min-h-0 flex-col overflow-hidden">
				<header className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
					<SidebarTrigger />
					<Separator orientation="vertical" className="h-5" />
					<div className="grid leading-tight">
						<span className="font-semibold">{VIEW_LABELS[view]}</span>
						<span className="text-muted-foreground">
							{view === "settings"
								? "Kubeconfig and connection"
								: view === "node"
									? "Cluster-wide"
									: "All namespaces"}
						</span>
					</div>
					<div className="ml-auto flex items-center gap-2">
						<Button
							onClick={() => run(PickKubeconfig)}
							disabled={busy}
							size="sm"
						>
							Choose kubeconfig
						</Button>
						<Button
							onClick={() => run(ResetKubeconfig)}
							disabled={busy}
							size="sm"
							variant="outline"
						>
							Auto
						</Button>
						{activeResource !== "" && (
							<Button
								onClick={reloadActive}
								disabled={busy}
								size="sm"
								variant="outline"
							>
								<RotateCw />
								Reload
							</Button>
						)}
					</div>
				</header>

				{error !== "" && (
					<div className="mx-4 mt-4 shrink-0 rounded-lg border border-foreground/30 bg-muted px-3 py-2">
						<p className="font-medium">Failed to load configuration</p>
						<p className="mt-1 break-words text-muted-foreground">{error}</p>
					</div>
				)}

				{view === "settings" ? (
					<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
						<section className="rounded-lg border p-4">
							<h2 className="font-medium">Connection</h2>
							<div className="mt-3 grid gap-2">
								<Field label="Source" value={sourceLabel(config?.source)} />
								<Field label="File" value={config?.path || "-"} mono />
								<Field label="Context" value={config?.context || "-"} />
								<Field label="Cluster" value={config?.cluster || "-"} />
								<Field label="Server" value={config?.server || "-"} mono />
							</div>
						</section>

						<section className="rounded-lg border p-4">
							<h2 className="font-medium">Resource</h2>
							<p className="mt-1 text-muted-foreground">
								Only the resource whose menu is open is fetched from the
								cluster.
							</p>
							<div className="mt-3 grid gap-2">
								<ResourceStatus label="Nodes" state={nodes} />
								<ResourceStatus label="Pods" state={pods} />
								<ResourceStatus label="Deployments" state={deployments} />
								<ResourceStatus label="StatefulSets" state={statefulSets} />
							</div>
						</section>

						{configMissing && config != null && (
							<section className="rounded-lg border p-4">
								<h2 className="font-medium">Kubeconfig not found</h2>
								<ul className="mt-3 space-y-1">
									{config.candidates.map((candidate) => (
										<li key={`${candidate.source}-${candidate.path}`}>
											<code className="font-mono">{candidate.path}</code>{" "}
											<span className="text-muted-foreground">
												({sourceLabel(candidate.source)}
												{candidate.exists ? ", exists" : ", missing"})
											</span>
										</li>
									))}
								</ul>
							</section>
						)}

						<p className="text-muted-foreground">
							No other settings available.
						</p>
					</div>
				) : (
					<>
						{view === "node" && (
							<ResourceTable
								accessors={NODE_ACCESSORS}
								columns={NODE_COLUMNS}
								error={nodes?.error ?? ""}
								groups={NODE_GROUPS}
								loaded={nodes?.loaded ?? false}
								loading={nodes?.loading ?? false}
								noun="Node"
								onRetry={reloadActive}
								rows={nodes?.items ?? NO_NODES}
							/>
						)}
						{view === "pod" && (
							<ResourceTable
								accessors={POD_ACCESSORS}
								columns={POD_COLUMNS}
								error={pods?.error ?? ""}
								groups={POD_GROUPS}
								loaded={pods?.loaded ?? false}
								loading={pods?.loading ?? false}
								noun="Pod"
								onRetry={reloadActive}
								rowActions={podRowActions}
								rows={pods?.items ?? NO_PODS}
							/>
						)}
						{view === "deployment" && (
							<ResourceTable
								accessors={DEPLOYMENT_ACCESSORS}
								columns={DEPLOYMENT_COLUMNS}
								error={deployments?.error ?? ""}
								groups={DEPLOYMENT_GROUPS}
								loaded={deployments?.loaded ?? false}
								loading={deployments?.loading ?? false}
								noun="Deployment"
								onRetry={reloadActive}
								rows={deployments?.items ?? NO_DEPLOYMENTS}
							/>
						)}
						{view === "statefulset" && (
							<ResourceTable
								accessors={STATEFULSET_ACCESSORS}
								columns={STATEFULSET_COLUMNS}
								error={statefulSets?.error ?? ""}
								groups={STATEFULSET_GROUPS}
								loaded={statefulSets?.loaded ?? false}
								loading={statefulSets?.loading ?? false}
								noun="StatefulSet"
								onRetry={reloadActive}
								rows={statefulSets?.items ?? NO_STATEFULSETS}
							/>
						)}
					</>
				)}
			</SidebarInset>

			<Drawer
				direction="right"
				dismissible={false}
				onOpenChange={(open) => {
					if (!open) {
						setYamlTarget(null);
					}
				}}
				open={yamlTarget != null}
			>
				<DrawerContent>
					{/*
					 * Closing goes straight through the controlled `open` state. With
					 * dismissible={false} vaul ignores its own close requests
					 * (`onOpenChange` returns early when open is false), so a DrawerClose
					 * button here would do nothing.
					 */}
					<Button
						aria-label="Close"
						className="absolute top-3 right-3"
						onClick={() => setYamlTarget(null)}
						size="icon-sm"
						variant="ghost"
					>
						<X />
					</Button>
					<DrawerHeader className="pr-12">
						<DrawerTitle>{yamlTarget?.name}</DrawerTitle>
						<DrawerDescription>
							{yamlTarget?.namespace} · Pod YAML
						</DrawerDescription>
					</DrawerHeader>
					{/* A right drawer spans the full height, so the editor fills the rest. */}
					<div className="min-h-0 flex-1 overflow-hidden border-t">
						{yamlLoading && (
							<p className="flex h-full items-center justify-center gap-2 text-muted-foreground">
								<LoaderCircle className="size-4 animate-spin" />
								Loading YAML…
							</p>
						)}
						{!yamlLoading && yamlError !== "" && (
							<p className="h-full overflow-auto p-6 text-center text-red-400">
								{yamlError}
							</p>
						)}
						{!yamlLoading && yamlError === "" && (
							<YAMLViewer document={yamlDocument} />
						)}
					</div>
				</DrawerContent>
			</Drawer>
		</SidebarProvider>
	);
}

// ResourceStatus summarises what the lazy fetch has done for one resource kind so far.
function ResourceStatus({
	label,
	state,
}: {
	label: string;
	state?: {
		items: unknown[];
		loaded: boolean;
		loading: boolean;
		error: string;
		updatedAt: string;
	};
}) {
	let detail = "not loaded yet";
	if (state != null) {
		if (state.loading && !state.loaded) {
			detail = "loading…";
		} else if (state.loaded && state.error === "") {
			detail = `${state.items.length} loaded, ${formatUpdatedAt(state.updatedAt)}`;
		} else if (state.loaded) {
			detail = `${state.items.length} loaded, ${state.error}`;
		} else if (state.error !== "") {
			detail = state.error;
		}
	}

	return <Field label={label} value={detail} />;
}

// The YAML viewer is read-only and monochrome: keys stay white while values step down to grey,
// so the drawer matches the app instead of pulling in CodeMirror's default colour palette.
// basicSetup registers its own highlight style as a fallback, so this one takes precedence.
const yamlHighlightStyle = HighlightStyle.define([
	{ tag: tags.propertyName, color: "var(--foreground)", fontWeight: "600" },
	{
		tag: [tags.string, tags.special(tags.string)],
		color: "var(--muted-foreground)",
	},
	{
		tag: [tags.number, tags.bool, tags.null, tags.atom],
		color: "var(--muted-foreground)",
	},
	{ tag: tags.comment, color: "var(--muted-foreground)", fontStyle: "italic" },
	{
		tag: [tags.punctuation, tags.separator, tags.bracket],
		color: "var(--muted-foreground)",
	},
]);

const yamlTheme = EditorView.theme(
	{
		"&": {
			height: "100%",
			backgroundColor: "transparent",
			color: "var(--foreground)",
			fontSize: "13px",
		},
		".cm-scroller": {
			fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
			lineHeight: "1.6",
		},
		".cm-gutters": {
			backgroundColor: "transparent",
			border: "none",
			color: "var(--muted-foreground)",
		},
		".cm-activeLine": {
			backgroundColor: "color-mix(in oklab, var(--foreground) 6%, transparent)",
		},
		".cm-activeLineGutter": {
			backgroundColor: "transparent",
			color: "var(--foreground)",
		},
		".cm-selectionBackground, ::selection": {
			backgroundColor:
				"color-mix(in oklab, var(--foreground) 22%, transparent)",
		},
	},
	{ dark: true },
);

const yamlExtensions = [
	basicSetup,
	yaml(),
	syntaxHighlighting(yamlHighlightStyle),
	EditorView.editable.of(false),
	yamlTheme,
];

// YAMLViewer mounts CodeMirror into a plain div and tears it down with the document, which is
// cheap because the drawer only ever shows one pod at a time.
function YAMLViewer({ document }: { document: string }) {
	const container = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const parent = container.current;
		if (parent == null) {
			return;
		}

		const view = new EditorView({
			doc: document,
			extensions: yamlExtensions,
			parent,
		});

		return () => view.destroy();
	}, [document]);

	return <div className="h-full overflow-hidden" ref={container} />;
}

function Field({
	label,
	value,
	mono,
}: {
	label: string;
	value: string;
	mono?: boolean;
}) {
	return (
		<span className="flex items-center gap-2">
			<span className="w-24 shrink-0 tracking-wide text-muted-foreground uppercase">
				{label}
			</span>
			<span className={mono ? "font-mono" : undefined}>{value}</span>
		</span>
	);
}

export default App;
