import { useCallback, useState } from "react";
import { Trash2 } from "lucide-react";
import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import { DeleteNamespaceDialog } from "@/components/delete-namespace-dialog";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";
import { NamespaceStatusLabel } from "@/components/status-label";
import { Button } from "@/components/ui/button";

const NAMESPACE_ACCESSORS: Accessors<kube.NamespaceInfo> = {
	name: (namespace) => namespace.name,
	key: (namespace) => namespace.name,
};

const NAMESPACE_GROUPS: GroupOption<kube.NamespaceInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "status", label: "Status", key: (namespace) => namespace.status },
];

const NAMESPACE_COLUMNS: Column<kube.NamespaceInfo>[] = [
	{
		header: "Name",
		className: "font-medium",
		render: (namespace) => namespace.name,
	},
	{
		header: "Status",
		render: (namespace) => (
			<NamespaceStatusLabel status={namespace.status} />
		),
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (namespace) => namespace.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_NAMESPACES: kube.NamespaceInfo[] = [];

// The namespaces menu is the one list that writes, so it is the one list with row selection: the
// ticked namespaces queue up for deletion and the dialog confirms them one at a time, each with
// its own name typed back. Everything else about the page is the shared table.
export function NamespacesPage() {
	const { state, reload } = useResource("namespaces");
	const [queue, setQueue] = useState<string[]>([]);
	const namespaces = state?.namespaces;

	// Deleting one namespace names the next one, so a multiple selection is confirmed namespace
	// by namespace instead of in one sweep nobody reads.
	const advance = useCallback(() => {
		setQueue((current) => current.slice(1));
	}, []);

	const cancel = useCallback(() => setQueue([]), []);

	// The watch delivers a removed namespace on its own, but reconnecting the list once the
	// queue is done is what puts the result on screen straight away.
	const handleDeleted = useCallback(() => {
		if (queue.length <= 1) {
			reload();
		}
		advance();
	}, [advance, queue.length, reload]);

	const target = queue[0] ?? null;

	return (
		<>
			<ResourceTable
				accessors={NAMESPACE_ACCESSORS}
				columns={NAMESPACE_COLUMNS}
				error={namespaces?.error ?? ""}
				groups={NAMESPACE_GROUPS}
				loaded={namespaces?.loaded ?? false}
				loading={namespaces?.loading ?? false}
				noun="Namespace"
				onRetry={reload}
				rows={namespaces?.items ?? NO_NAMESPACES}
				selectable
				toolbar={(selected) => (
					<>
						<Button
							disabled={selected.length === 0}
							onClick={() =>
								setQueue(selected.map((namespace) => namespace.name))
							}
							size="sm"
							variant="outline"
						>
							<Trash2 />
							Delete
						</Button>
						<span className="text-muted-foreground">
							{selected.length} selected
						</span>
					</>
				)}
			/>

			{target != null && (
				<DeleteNamespaceDialog
					key={target}
					name={target}
					onCancel={cancel}
					onDeleted={handleDeleted}
					remaining={queue.length - 1}
				/>
			)}
		</>
	);
}
