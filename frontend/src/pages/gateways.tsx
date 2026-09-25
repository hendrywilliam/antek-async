import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetGatewayYAML } from "../../wailsjs/go/main/App";
import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";
import { ConditionLabel } from "@/components/status-label";
import { YamlDrawer } from "@/components/yaml-drawer";
import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const GATEWAY_ACCESSORS: Accessors<kube.GatewayInfo> = {
	namespace: (gateway) => gateway.namespace,
	name: (gateway) => gateway.name,
	key: (gateway) => `${gateway.namespace}/${gateway.name}`,
};

const GATEWAY_GROUPS: GroupOption<kube.GatewayInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (gateway) => gateway.namespace },
	{ value: "class", label: "Class", key: (gateway) => gateway.class },
];

const GATEWAY_COLUMNS: Column<kube.GatewayInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (gateway) => gateway.namespace,
	},
	{
		header: "Name",
		className: "font-medium",
		render: (gateway) => gateway.name,
	},
	{ header: "Class", render: (gateway) => gateway.class },
	{
		header: "Address",
		className: "text-muted-foreground",
		render: (gateway) => gateway.address,
	},
	{
		header: "Programmed",
		render: (gateway) => <ConditionLabel status={gateway.programmed} />,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (gateway) => gateway.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_GATEWAYS: kube.GatewayInfo[] = [];

export function GatewaysPage() {
	const { state, reload } = useResource("gateways");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

	// The row ends in its own actions: View YAML opens the drawer for that one object.
	const gatewayRowActions = useCallback(
		(gateway: kube.GatewayInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${gateway.name}`}
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
								namespace: gateway.namespace,
								name: gateway.name,
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

	const gateways = state?.gateways;

	return (
		<>
			<ResourceTable
				accessors={GATEWAY_ACCESSORS}
				columns={GATEWAY_COLUMNS}
				error={gateways?.error ?? ""}
				groups={GATEWAY_GROUPS}
				loaded={gateways?.loaded ?? false}
				loading={gateways?.loading ?? false}
				noun="Gateway"
				onRetry={reload}
				rowActions={gatewayRowActions}
				rows={gateways?.items ?? NO_GATEWAYS}
			/>
			<YamlDrawer
				fetchYaml={GetGatewayYAML}
				noun="Gateway"
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
