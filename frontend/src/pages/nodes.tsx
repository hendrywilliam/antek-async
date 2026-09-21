import { useEffect, useMemo, useState } from "react";
import { GetNodeUsages } from "../../wailsjs/go/main/App";
import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";
import { NodeStatusLabel } from "@/components/status-label";
import { UsageCell } from "@/components/usage-cell";

const NODE_ACCESSORS: Accessors<kube.NodeInfo> = {
	name: (node) => node.name,
	key: (node) => node.name,
};

const NODE_GROUPS: GroupOption<kube.NodeInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "status", label: "Status", key: (node) => node.status },
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

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_NODES: kube.NodeInfo[] = [];

// How often the page asks metrics-server for CPU and memory. It is not part of the watch, because
// that API has no watch and pushes nothing.
const USAGE_INTERVAL = 5000;

export function NodesPage() {
	const { state, reload } = useResource("nodes");
	const nodes = state?.nodes;
	const [usage, setUsage] = useState<kube.NodeUsage[]>([]);
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
				const next = await GetNodeUsages();
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
	// never disturb a filter.
	const usageByNode = useMemo(
		() => new Map(usage.map((item) => [item.name, item] as const)),
		[usage],
	);

	// A node has no Ready column, so the metrics sit right after Status, which is the column that
	// says whether the node is healthy. That is why the columns are built per snapshot rather
	// than once at module level.
	const columns = useMemo<Column<kube.NodeInfo>[]>(() => {
		const usageColumn: Column<kube.NodeInfo> = {
			header: "CPU / Memory",
			align: "right",
			render: (node) => <UsageCell usage={usageByNode.get(node.name)} />,
		};

		// Found by header rather than by index, so reordering the columns above cannot quietly
		// move the metrics somewhere else.
		const after = NODE_COLUMNS.findIndex((column) => column.header === "Status") + 1;

		return [
			...NODE_COLUMNS.slice(0, after),
			usageColumn,
			...NODE_COLUMNS.slice(after),
		];
	}, [usageByNode]);

	return (
		<ResourceTable
			accessors={NODE_ACCESSORS}
			columns={columns}
			error={nodes?.error ?? ""}
			groups={NODE_GROUPS}
			loaded={nodes?.loaded ?? false}
			loading={nodes?.loading ?? false}
			notice={usageError}
			noun="Node"
			onRetry={reload}
			rows={nodes?.items ?? NO_NODES}
		/>
	);
}
