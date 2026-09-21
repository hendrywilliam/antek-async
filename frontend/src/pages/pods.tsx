import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
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

	return (
		<>
			<ResourceTable
				accessors={POD_ACCESSORS}
				columns={POD_COLUMNS}
				error={pods?.error ?? ""}
				groups={POD_GROUPS}
				loaded={pods?.loaded ?? false}
				loading={pods?.loading ?? false}
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
