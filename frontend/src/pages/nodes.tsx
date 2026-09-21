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

export function NodesPage() {
	const { state, reload } = useResource("nodes");
	const nodes = state?.nodes;

	return (
		<ResourceTable
			accessors={NODE_ACCESSORS}
			columns={NODE_COLUMNS}
			error={nodes?.error ?? ""}
			groups={NODE_GROUPS}
			loaded={nodes?.loaded ?? false}
			loading={nodes?.loading ?? false}
			noun="Node"
			onRetry={reload}
			rows={nodes?.items ?? NO_NODES}
		/>
	);
}
