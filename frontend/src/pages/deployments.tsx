import { kube } from "../../wailsjs/go/models";
import { useResource } from "@/use-resource";
import {
	type Accessors,
	type Column,
	namespaceGroups,
	ResourceTable,
} from "@/components/resource-table";

const DEPLOYMENT_ACCESSORS: Accessors<kube.DeploymentInfo> = {
	namespace: (deployment) => deployment.namespace,
	name: (deployment) => deployment.name,
	key: (deployment) => `${deployment.namespace}/${deployment.name}`,
};

const DEPLOYMENT_GROUPS = namespaceGroups<kube.DeploymentInfo>(
	(deployment) => deployment.namespace,
);

const DEPLOYMENT_COLUMNS: Column<kube.DeploymentInfo>[] = [
	{
		header: "Namespace",
		className: "text-muted-foreground",
		render: (item) => item.namespace,
	},
	{ header: "Name", className: "font-medium", render: (item) => item.name },
	{ header: "Ready", align: "right", render: (item) => item.ready },
	{ header: "Up-to-date", align: "right", render: (item) => item.upToDate },
	{ header: "Available", align: "right", render: (item) => item.available },
	{
		header: "Age",
		className: "text-muted-foreground",
		render: (item) => item.age,
	},
];

// Stable empty slices keep ResourceTable's useMemo dependencies from changing every render.
const NO_DEPLOYMENTS: kube.DeploymentInfo[] = [];

export function DeploymentsPage() {
	const { state, reload } = useResource("deployments");
	const deployments = state?.deployments;

	return (
		<ResourceTable
			accessors={DEPLOYMENT_ACCESSORS}
			columns={DEPLOYMENT_COLUMNS}
			error={deployments?.error ?? ""}
			groups={DEPLOYMENT_GROUPS}
			loaded={deployments?.loaded ?? false}
			loading={deployments?.loading ?? false}
			noun="Deployment"
			onRetry={reload}
			rows={deployments?.items ?? NO_DEPLOYMENTS}
		/>
	);
}
