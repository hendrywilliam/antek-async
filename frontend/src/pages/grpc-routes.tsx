import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetGRPCRouteYAML } from "../../wailsjs/go/main/App";
import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";
import { YamlDrawer } from "@/components/yaml-drawer";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const GRPC_ROUTE_ACCESSORS: Accessors<kube.GRPCRouteInfo> = {
	namespace: (route) => route.namespace,
	name: (route) => route.name,
	key: (route) => `${route.namespace}/${route.name}`,
};

const GRPC_ROUTE_GROUPS: GroupOption<kube.GRPCRouteInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{
		value: "namespace",
		label: "Namespace",
		key: (route) => route.namespace,
	},
	// Grouping by the referenced parent is what makes "every route attached to this Gateway" one
	// readable bucket, so the value is the resolved namespace/name pair rather than a bare name.
	{ value: "parentRefs", label: "Parent", key: (route) => route.parentRefs },
];

const GRPC_ROUTE_COLUMNS: Column<kube.GRPCRouteInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (route) => route.namespace,
	},
	{
		header: "Name",
		className: "font-medium",
		render: (route) => route.name,
	},
	{
		header: "Hostnames",
		render: (route) => route.hostnames,
	},
	{
		header: "Parent refs",
		className: "text-muted-foreground",
		render: (route) => route.parentRefs,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (route) => route.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_GRPC_ROUTES: kube.GRPCRouteInfo[] = [];

export function GRPCRoutesPage() {
	const { state, reload } = useResource("grpcRoutes");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

	// The row ends in its own actions: View YAML opens the drawer for that one object.
	const grpcRouteRowActions = useCallback(
		(route: kube.GRPCRouteInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${route.name}`}
						size="icon-sm"
						variant="ghost"
					>
						<MoreHorizontal />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						onSelect={() =>
							setYamlTarget({
								namespace: route.namespace,
								name: route.name,
							})
						}
					>
						View YAML
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		),
		[],
	);

	const routes = state?.grpcRoutes;

	return (
		<>
			<ResourceTable
				accessors={GRPC_ROUTE_ACCESSORS}
				columns={GRPC_ROUTE_COLUMNS}
				error={routes?.error ?? ""}
				groups={GRPC_ROUTE_GROUPS}
				loaded={routes?.loaded ?? false}
				loading={routes?.loading ?? false}
				noun="GRPCRoute"
				onRetry={reload}
				rowActions={grpcRouteRowActions}
				rows={routes?.items ?? NO_GRPC_ROUTES}
			/>
			<YamlDrawer
				fetchYaml={GetGRPCRouteYAML}
				noun="GRPCRoute"
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
