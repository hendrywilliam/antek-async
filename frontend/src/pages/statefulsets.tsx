import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	namespaceGroups,
	ResourceTable,
} from "@/components/resource-table";

const STATEFULSET_ACCESSORS: Accessors<kube.StatefulSetInfo> = {
	namespace: (statefulSet) => statefulSet.namespace,
	name: (statefulSet) => statefulSet.name,
	key: (statefulSet) => `${statefulSet.namespace}/${statefulSet.name}`,
};

const STATEFULSET_GROUPS = namespaceGroups<kube.StatefulSetInfo>(
	(statefulSet) => statefulSet.namespace,
);

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

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_STATEFULSETS: kube.StatefulSetInfo[] = [];

export function StatefulSetsPage() {
	const { state, reload } = useResource("statefulSets");
	const statefulSets = state?.statefulSets;

	return (
		<ResourceTable
			accessors={STATEFULSET_ACCESSORS}
			columns={STATEFULSET_COLUMNS}
			error={statefulSets?.error ?? ""}
			groups={STATEFULSET_GROUPS}
			loaded={statefulSets?.loaded ?? false}
			loading={statefulSets?.loading ?? false}
			noun="StatefulSet"
			onRetry={reload}
			rows={statefulSets?.items ?? NO_STATEFULSETS}
		/>
	);
}
