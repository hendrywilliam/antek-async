import { useApp } from "@/app-context";

const SOURCE_LABELS: Record<string, string> = {
	manual: "picked manually",
	KUBECONFIG: "env KUBECONFIG",
	home: "~/.kube/config",
	project: "<cwd>/.kube/config",
	none: "not found",
};

function sourceLabel(source?: string): string {
	if (!source) {
		return "-";
	}
	return SOURCE_LABELS[source] ?? source;
}

function formatUpdatedAt(value?: string): string {
	if (!value) {
		return "-";
	}
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? "-" : date.toLocaleTimeString();
}

// ResourceStatus summarises what the lazy fetch has done for one resource kind so far.
function ResourceStatus({
	label,
	state,
}: {
	label: string;
	state?: {
		items: unknown[];
		loaded: boolean;
		loading: boolean;
		error: string;
		updatedAt: string;
	};
}) {
	let detail = "not loaded yet";
	if (state != null) {
		if (state.loading && !state.loaded) {
			detail = "loading…";
		} else if (state.loaded && state.error === "") {
			detail = `${state.items.length} loaded, ${formatUpdatedAt(state.updatedAt)}`;
		} else if (state.loaded) {
			detail = `${state.items.length} loaded, ${state.error}`;
		} else if (state.error !== "") {
			detail = state.error;
		}
	}

	return <Field label={label} value={detail} />;
}

function Field({
	label,
	value,
	mono,
}: {
	label: string;
	value: string;
	mono?: boolean;
}) {
	return (
		<span className="flex items-center gap-2">
			<span className="w-24 shrink-0 tracking-wide text-muted-foreground uppercase">
				{label}
			</span>
			<span className={mono ? "font-mono" : undefined}>{value}</span>
		</span>
	);
}

export function SettingsPage() {
	const { state } = useApp();

	const config = state?.config;
	const configMissing = config != null && config.path === "";

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
			<section className="rounded-lg border p-4">
				<h2 className="font-medium">Connection</h2>
				<div className="mt-3 grid gap-2">
					<Field label="Source" value={sourceLabel(config?.source)} />
					<Field label="File" value={config?.path || "-"} mono />
					<Field label="Context" value={config?.context || "-"} />
					<Field label="Cluster" value={config?.cluster || "-"} />
					<Field label="Server" value={config?.server || "-"} mono />
				</div>
			</section>

			<section className="rounded-lg border p-4">
				<h2 className="font-medium">Resource</h2>
				<p className="mt-1 text-muted-foreground">
					Only the resource whose menu is open is fetched from the cluster.
				</p>
				<div className="mt-3 grid gap-2">
					<ResourceStatus label="Nodes" state={state?.nodes} />
					<ResourceStatus label="Namespaces" state={state?.namespaces} />
					<ResourceStatus label="Pods" state={state?.pods} />
					<ResourceStatus label="Deployments" state={state?.deployments} />
					<ResourceStatus label="StatefulSets" state={state?.statefulSets} />
					<ResourceStatus label="Services" state={state?.services} />
					<ResourceStatus
						label="GatewayClass"
						state={state?.gatewayClasses}
					/>
					<ResourceStatus label="Gateway" state={state?.gateways} />
					<ResourceStatus label="HTTPRoute" state={state?.httpRoutes} />
					<ResourceStatus label="GRPCRoute" state={state?.grpcRoutes} />
				</div>
			</section>

			{configMissing && config != null && (
				<section className="rounded-lg border p-4">
					<h2 className="font-medium">Kubeconfig not found</h2>
					<ul className="mt-3 space-y-1">
						{config.candidates.map((candidate) => (
							<li key={`${candidate.source}-${candidate.path}`}>
								<code className="font-mono">{candidate.path}</code>{" "}
								<span className="text-muted-foreground">
									({sourceLabel(candidate.source)}
									{candidate.exists ? ", exists" : ", missing"})
								</span>
							</li>
						))}
					</ul>
				</section>
			)}

			<p className="text-muted-foreground">No other settings available.</p>
		</div>
	);
}
