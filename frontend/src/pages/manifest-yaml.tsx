import { LoaderCircle } from "lucide-react";
import { type ReactNode, useCallback, useState } from "react";
import { ApplyYAML } from "../../wailsjs/go/main/App";
import { kube } from "../../wailsjs/go/models";
import { useApp } from "@/app-context";
import { Button } from "@/components/ui/button";
import { YamlEditor } from "@/components/yaml-editor";

// The manifest editor always starts empty; clearing it remounts the component rather than pushing a
// new document in, because the live value must never be fed back as the seed.
const EMPTY_DOCUMENT = "";

// describeTarget names what was applied the way kubectl prints it, leaving the namespace out for
// cluster-scoped kinds because the API server returns none for them.
function describeTarget(target: kube.ApplyResult): string {
	return target.namespace === ""
		? `${target.kind} ${target.name}`
		: `${target.kind} ${target.namespace}/${target.name}`;
}

// The manifest page streams no cluster kind, so it reads the shared state directly, the same way the
// Settings page does. Its own outcome (applied or failed) stays local: a manifest the API server
// rejected is not a configuration error for the whole shell.
export function ManifestYamlPage() {
	const { state } = useApp();

	const [document, setDocument] = useState("");
	const [applying, setApplying] = useState(false);
	const [result, setResult] = useState<kube.ApplyResult | null>(null);
	const [error, setError] = useState("");

	// Changing the key is what empties the editor: YamlEditor builds its view once per mount.
	const [resetKey, setResetKey] = useState(0);

	const kubeconfigMissing = state?.config?.path === "";
	const canApply = document.trim() !== "" && !applying && !kubeconfigMissing;

	const apply = useCallback(async () => {
		setApplying(true);
		setError("");
		setResult(null);
		try {
			setResult(await ApplyYAML(document));
		} catch (err) {
			setError(String(err));
		} finally {
			setApplying(false);
		}
	}, [document]);

	const clear = useCallback(() => {
		setDocument("");
		setResult(null);
		setError("");
		setResetKey((key) => key + 1);
	}, []);

	let status: ReactNode = null;
	if (kubeconfigMissing) {
		status = (
			<p className="text-muted-foreground">
				No kubeconfig found. Choose one in Settings.
			</p>
		);
	} else if (error !== "") {
		status = <p className="break-words text-red-400">{error}</p>;
	} else if (result != null) {
		status = <p>Applied {describeTarget(result)}</p>;
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<div className="flex shrink-0 flex-wrap items-center gap-3 border-b px-4 py-3">
				<div className="grid leading-tight">
					<span className="font-medium">Server-side apply</span>
					<span className="text-muted-foreground">
						One manifest at a time, any kind the cluster knows
					</span>
				</div>
				<div className="ml-auto flex items-center gap-2">
					<Button disabled={!canApply} onClick={apply} size="sm">
						{applying && <LoaderCircle className="animate-spin" />}
						Apply
					</Button>
					<Button
						disabled={applying}
						onClick={clear}
						size="sm"
						variant="outline"
					>
						Clear
					</Button>
				</div>
			</div>

			{status != null && (
				<div className="shrink-0 border-b px-4 py-2">{status}</div>
			)}

			{/* The editor owns the rest of the column and scrolls inside CodeMirror. */}
			<div className="min-h-0 flex-1 overflow-hidden">
				<YamlEditor
					initialDocument={EMPTY_DOCUMENT}
					key={resetKey}
					onChange={setDocument}
				/>
			</div>
		</div>
	);
}
