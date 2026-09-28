import { useCallback, useState } from "react";
import { MoreHorizontal } from "lucide-react";
import { GetEndpointSliceYAML } from "../../wailsjs/go/main/App";
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

const ENDPOINT_SLICE_ACCESSORS: Accessors<kube.EndpointSliceInfo> = {
	namespace: (slice) => slice.namespace,
	name: (slice) => slice.name,
	key: (slice) => `${slice.namespace}/${slice.name}`,
};

const ENDPOINT_SLICE_GROUPS: GroupOption<kube.EndpointSliceInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (slice) => slice.namespace },
	// One service is normally split across an IPv4 and an IPv6 slice, so this is the grouping
	// that shows the two halves of the same thing side by side.
	{ value: "addressType", label: "Address type", key: (slice) => slice.addressType },
];

const ENDPOINT_SLICE_COLUMNS: Column<kube.EndpointSliceInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (slice) => slice.namespace,
	},
	{
		header: "Name",
		className: "font-medium",
		render: (slice) => slice.name,
	},
	{ header: "Address Type", render: (slice) => slice.addressType },
	{
		header: "Port(s)",
		className: "text-muted-foreground",
		render: (slice) => slice.ports,
	},
	{
		header: "Endpoints",
		className: "text-muted-foreground",
		render: (slice) => slice.endpoints,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (slice) => slice.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_ENDPOINT_SLICES: kube.EndpointSliceInfo[] = [];

export function EndpointSlicesPage() {
	const { state, reload } = useResource("endpointSlices");
	const [yamlTarget, setYamlTarget] = useState<{
		namespace: string;
		name: string;
	} | null>(null);

	// The row ends in its own actions: View YAML opens the drawer for that one object.
	const endpointSliceRowActions = useCallback(
		(slice: kube.EndpointSliceInfo) => (
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						aria-label={`Actions for ${slice.name}`}
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
								namespace: slice.namespace,
								name: slice.name,
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

	const endpointSlices = state?.endpointSlices;

	return (
		<>
			<ResourceTable
				accessors={ENDPOINT_SLICE_ACCESSORS}
				columns={ENDPOINT_SLICE_COLUMNS}
				error={endpointSlices?.error ?? ""}
				groups={ENDPOINT_SLICE_GROUPS}
				loaded={endpointSlices?.loaded ?? false}
				loading={endpointSlices?.loading ?? false}
				noun="EndpointSlice"
				onRetry={reload}
				rowActions={endpointSliceRowActions}
				rows={endpointSlices?.items ?? NO_ENDPOINT_SLICES}
			/>
			<YamlDrawer
				fetchYaml={GetEndpointSliceYAML}
				noun="EndpointSlice"
				onClose={() => setYamlTarget(null)}
				target={yamlTarget}
			/>
		</>
	);
}
