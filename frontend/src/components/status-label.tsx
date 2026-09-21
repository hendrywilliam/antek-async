// Status is plain coloured text rather than a badge: green while healthy, amber while the
// pod is still coming up, and red for failures such as CrashLoopBackOff, ImagePullBackOff,
// Error, Failed or Evicted. Nothing else in the UI uses colour.
const OK_STATUSES = new Set(["Running", "Succeeded", "Completed"]);
const WARN_STATUSES = new Set([
	"Pending",
	"ContainerCreating",
	"PodInitializing",
	"NotReady",
	"Terminating",
]);

const TONE_CLASSES = {
	ok: "text-emerald-400",
	warn: "text-amber-400",
	bad: "text-red-400",
} as const;

function statusTone(status: string): "ok" | "warn" | "bad" {
	if (OK_STATUSES.has(status)) {
		return "ok";
	}
	if (WARN_STATUSES.has(status) || status.startsWith("Init:")) {
		return "warn";
	}
	return "bad";
}

export function StatusLabel({ status }: { status: string }) {
	return <span className={TONE_CLASSES[statusTone(status)]}>{status}</span>;
}

// Nodes reuse the coloured text, but a node that is NotReady is a real failure rather than a
// workload still coming up, so the mapping is its own.
export function NodeStatusLabel({ status }: { status: string }) {
	const tone =
		status === "Ready" ? "ok" : status === "NotReady" ? "bad" : "warn";

	return <span className={TONE_CLASSES[tone]}>{status}</span>;
}

// A namespace is Active for its whole life, and Terminating while its finalizers run, so it
// gets its own mapping too.
export function NamespaceStatusLabel({ status }: { status: string }) {
	const tone =
		status === "Active" ? "ok" : status === "Terminating" ? "warn" : "bad";

	return <span className={TONE_CLASSES[tone]}>{status}</span>;
}
