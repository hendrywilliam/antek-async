import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	type GroupOption,
	NO_GROUPING,
	ResourceTable,
} from "@/components/resource-table";

const SERVICE_ACCESSORS: Accessors<kube.ServiceInfo> = {
	namespace: (service) => service.namespace,
	name: (service) => service.name,
	key: (service) => `${service.namespace}/${service.name}`,
};

const SERVICE_GROUPS: GroupOption<kube.ServiceInfo>[] = [
	{ value: NO_GROUPING, label: "No grouping", key: () => "" },
	{ value: "namespace", label: "Namespace", key: (service) => service.namespace },
	{ value: "type", label: "Type", key: (service) => service.type },
];

const SERVICE_COLUMNS: Column<kube.ServiceInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (service) => service.namespace,
	},
	{
		header: "Name",
		className: "font-medium",
		render: (service) => service.name,
	},
	{ header: "Type", render: (service) => service.type },
	{
		header: "Cluster IP",
		className: "text-muted-foreground",
		render: (service) => service.clusterIP,
	},
	{
		header: "External IP",
		className: "text-muted-foreground",
		render: (service) => service.externalIP,
	},
	{
		header: "Port(s)",
		className: "text-muted-foreground",
		render: (service) => service.ports,
	},
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (service) => service.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_SERVICES: kube.ServiceInfo[] = [];

export function ServicesPage() {
	const { state, reload } = useResource("services");
	const services = state?.services;

	return (
		<ResourceTable
			accessors={SERVICE_ACCESSORS}
			columns={SERVICE_COLUMNS}
			error={services?.error ?? ""}
			groups={SERVICE_GROUPS}
			loaded={services?.loaded ?? false}
			loading={services?.loading ?? false}
			noun="Service"
			onRetry={reload}
			rows={services?.items ?? NO_SERVICES}
		/>
	);
}
