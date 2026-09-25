import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetGatewayClassYAML } from "../../wailsjs/go/main/App";
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

// A module-level fetcher keeps its identity stable, so the drawer's effect does not re-run on every
// render. GatewayClass is cluster scoped, so the namespace it is handed is always empty.
const fetchGatewayClassYaml = (_namespace: string, name: string) =>
	GetGatewayClassYAML(name);

// GatewayClass is cluster scoped, so the accessors omit namespace and the table drops the
// namespace filter and grouping the same way Nodes and Namespaces do.
const GATEWAY_CLASS_ACCESSORS: Accessors<kube.GatewayClassInfo> = {
	name: (gatewayClass) => gatewayClass.name,
	key: (gatewayClass) => gatewayClass.name,
};

const GATEWAY_CLASS_GROUPS: GroupOption<kube.GatewayClassInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{
		value: "accepted",
		label: "Accepted",
		key: (gatewayClass) => gatewayClass.accepted,
	},
];

const GATEWAY_CLASS_COLUMNS: Column<kube.GatewayClassInfo>[] = [
	{
		header: "Name",
		className: "font-medium",
		render: (gatewayClass) => gatewayClass.name,
	},
	{
		header: "Controller",
		className: "text-muted-foreground",
		render: (gatewayClass) => gatewayClass.controller,
	},
	{
		header: "Accepted",
		render: (gatewayClass) => (
			<ConditionLabel status={gatewayClass.accepted} />
		),
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (gatewayClass) => gatewayClass.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_GATEWAY_CLASSES: kube.GatewayClassInfo[] = [];

export function GatewayClassesPage() {
	const { state, reload } = useResource("gatewayClasses");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

	// The row ends in its own actions: View YAML opens the drawer for that one object.
	const gatewayClassRowActions = useCallback(
		(gatewayClass: kube.GatewayClassInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${gatewayClass.name}`}
						size="icon-sm"
						variant="ghost"
					>
						<MoreHorizontal />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="end">
					<DropdownMenuItem
						onSelect={() =>
							setYamlTarget({ namespace: "", name: gatewayClass.name })
						}
					>
						View YAML
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		),
		[],
	);

	const gatewayClasses = state?.gatewayClasses;

	return (
		<>
			<ResourceTable
				accessors={GATEWAY_CLASS_ACCESSORS}
				columns={GATEWAY_CLASS_COLUMNS}
				error={gatewayClasses?.error ?? ""}
				groups={GATEWAY_CLASS_GROUPS}
				loaded={gatewayClasses?.loaded ?? false}
				loading={gatewayClasses?.loading ?? false}
				noun="GatewayClass"
				onRetry={reload}
				rowActions={gatewayClassRowActions}
				rows={gatewayClasses?.items ?? NO_GATEWAY_CLASSES}
			/>
			<YamlDrawer
				fetchYaml={fetchGatewayClassYaml}
				noun="GatewayClass"
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
