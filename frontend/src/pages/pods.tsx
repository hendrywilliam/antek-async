import { useCallback, useEffect, useMemo, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetPodUsages } from "../../wailsjs/go/main/App";
import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import { PodYamlDrawer } from "@/components/pod-yaml-drawer";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";
import { StatusLabel } from "@/components/status-label";
import { UsageCell } from "@/components/usage-cell";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const POD_ACCESSORS: Accessors<kube.PodInfo> = {
	namespace: (pod) => pod.namespace,
	name: (pod) => pod.name,
	key: (pod) => `${pod.namespace}/${pod.name}`,
};

const POD_GROUPS: GroupOption<kube.PodInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (pod) => pod.namespace },
	{ value: "node", label: "Node", key: (pod) => pod.node },
	{ value: "status", label: "Status", key: (pod) => pod.status },
];

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

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_PODS: kube.PodInfo[] = [];

// How often the page asks metrics-server for CPU and memory. It is not part of the watch, because
// that API has no watch and pushes nothing.
const USAGE_INTERVAL = 5000;

export function PodsPage() {
	const { state, reload } = useResource("pods");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

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
					<DropdownMenuItem
						onSelect={() =>
							setYamlTarget({ namespace: pod.namespace, name: pod.name })
						}
					>
						View YAML
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		),
		[],
	);

	const pods = state?.pods;
	const [usage, setUsage] = useState<kube.PodUsage[]>([]);
	const [usageError, setUsageError] = useState("");

	// Ask for CPU and memory on a plain interval, for as long as the page is open. The call is
	// awaited inside a try because a call that throws or rejects has to land somewhere visible: an
	// unnoticed one leaves the column empty with no explanation for it.
	useEffect(() => {
		let cancelled = false;
		let inFlight = false;

		// Numbers on screen belong to the kubeconfig they came from.
		setUsage([]);
		setUsageError("");

		const ask = async () => {
			// A slow API server must not stack requests.
			if (inFlight) {
				return;
			}
			inFlight = true;

			try {
				const next = await GetPodUsages();
				if (!cancelled) {
					setUsage(next ?? []);
					setUsageError("");
				}
			} catch (err: unknown) {
				if (!cancelled) {
					setUsage([]);
					setUsageError(String(err));
				}
			} finally {
				inFlight = false;
			}
		};

		ask();
		const timer = window.setInterval(ask, USAGE_INTERVAL);

		return () => {
			cancelled = true;
			window.clearInterval(timer);
		};
	}, [state?.config?.path]);

	// The lookup is rebuilt from each snapshot instead of being kept per row, so a fresh poll can
	// never disturb a filter or a tick.
	const usageByPod = useMemo(
		() =>
			new Map(
				usage.map((item) => [`${item.namespace}/${item.name}`, item] as const),
			),
		[usage],
	);

	// CPU and memory belong with the rest of the pod's numbers, so the column sits right after
	// Ready. That is why the columns are built per snapshot rather than once at module level.
	const columns = useMemo<Column<kube.PodInfo>[]>(() => {
		const usageColumn: Column<kube.PodInfo> = {
			header: "CPU / Memory",
			align: "right",
			render: (pod) => (
				<UsageCell usage={usageByPod.get(`${pod.namespace}/${pod.name}`)} />
			),
		};

		// Found by header rather than by index, so reordering the columns above cannot quietly
		// move the metrics somewhere else.
		const after = POD_COLUMNS.findIndex((column) => column.header === "Ready") + 1;

		return [
			...POD_COLUMNS.slice(0, after),
			usageColumn,
			...POD_COLUMNS.slice(after),
		];
	}, [usageByPod]);

	return (
		<>
			<ResourceTable
				accessors={POD_ACCESSORS}
				columns={columns}
				error={pods?.error ?? ""}
				groups={POD_GROUPS}
				loaded={pods?.loaded ?? false}
				loading={pods?.loading ?? false}
				notice={usageError}
				noun="Pod"
				onRetry={reload}
				rowActions={podRowActions}
				rows={pods?.items ?? NO_PODS}
			/>
			<PodYamlDrawer
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
