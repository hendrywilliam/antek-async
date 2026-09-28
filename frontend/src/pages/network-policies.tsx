import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetNetworkPolicyYAML } from "../../wailsjs/go/main/App";
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

const NETWORK_POLICY_ACCESSORS: Accessors<kube.NetworkPolicyInfo> = {
	namespace: (policy) => policy.namespace,
	name: (policy) => policy.name,
	key: (policy) => `${policy.namespace}/${policy.name}`,
};

const NETWORK_POLICY_GROUPS: GroupOption<kube.NetworkPolicyInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (policy) => policy.namespace },
	// Policies that select the same pods are the ones worth reading together, so the selector is
	// also a grouping: "<none>" is the bucket of policies that cover a whole namespace.
	{
		value: "podSelector",
		label: "Pod selector",
		key: (policy) => policy.podSelector,
	},
];

const NETWORK_POLICY_COLUMNS: Column<kube.NetworkPolicyInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (policy) => policy.namespace,
	},
	{
		header: "Name",
		className: "font-medium",
		render: (policy) => policy.name,
	},
	{
		header: "Pod Selector",
		className: "text-muted-foreground",
		render: (policy) => policy.podSelector,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (policy) => policy.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_NETWORK_POLICIES: kube.NetworkPolicyInfo[] = [];

export function NetworkPoliciesPage() {
	const { state, reload } = useResource("networkPolicies");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

	// The row ends in its own actions: View YAML opens the drawer for that one object.
	const networkPolicyRowActions = useCallback(
		(policy: kube.NetworkPolicyInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${policy.name}`}
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
								namespace: policy.namespace,
								name: policy.name,
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

	const networkPolicies = state?.networkPolicies;

	return (
		<>
			<ResourceTable
				accessors={NETWORK_POLICY_ACCESSORS}
				columns={NETWORK_POLICY_COLUMNS}
				error={networkPolicies?.error ?? ""}
				groups={NETWORK_POLICY_GROUPS}
				loaded={networkPolicies?.loaded ?? false}
				loading={networkPolicies?.loading ?? false}
				noun="NetworkPolicy"
				onRetry={reload}
				rowActions={networkPolicyRowActions}
				rows={networkPolicies?.items ?? NO_NETWORK_POLICIES}
			/>
			<YamlDrawer
				fetchYaml={GetNetworkPolicyYAML}
				noun="NetworkPolicy"
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
