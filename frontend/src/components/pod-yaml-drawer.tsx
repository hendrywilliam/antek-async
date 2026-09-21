import { LoaderCircle, X } from "lucide-react";
import { useEffect, useState } from "react";
import { GetPodYAML } from "../../wailsjs/go/main/App";
import { Button } from "@/components/ui/button";
import {
	Drawer,
	DrawerContent,
	DrawerDescription,
	DrawerHeader,
	DrawerTitle,
} from "@/components/ui/drawer";
import { YamlViewer } from "@/components/yaml-viewer";

// PodYamlDrawer fetches a single pod on demand and shows it as read-only YAML, so it is
// independent of the active watch and can be opened from any menu. Pass a null `target` to
// keep it closed and call `onClose` to dismiss it.
export function PodYamlDrawer({
	target,
	onClose,
}: {
	target: { namespace: string; name: string } | null;
	onClose: () => void;
}) {
	const namespace = target?.namespace ?? "";
	const name = target?.name ?? "";

	const [document, setDocument] = useState("");
	const [error, setError] = useState("");
	const [loading, setLoading] = useState(false);

	useEffect(() => {
		if (namespace === "" || name === "") {
			return;
		}

		let cancelled = false;
		setDocument("");
		setError("");
		setLoading(true);

		GetPodYAML(namespace, name)
			.then((result) => {
				if (!cancelled) {
					setDocument(result);
				}
			})
			.catch((err) => {
				if (!cancelled) {
					setError(String(err));
				}
			})
			.finally(() => {
				if (!cancelled) {
					setLoading(false);
				}
			});

		return () => {
			cancelled = true;
		};
	}, [namespace, name]);

	return (
		<Drawer
			direction="right"
			dismissible={false}
			onOpenChange={(open) => {
				if (!open) {
					onClose();
				}
			}}
			open={target != null}
		>
			<DrawerContent>
				{/*
				 * Closing goes straight through the controlled `open` state. With
				 * dismissible={false} vaul ignores its own close requests
				 * (`onOpenChange` returns early when open is false), so a DrawerClose
				 * button here would do nothing.
				 */}
				<Button
					aria-label="Close"
					className="absolute top-3 right-3"
					onClick={onClose}
					size="icon-sm"
					variant="ghost"
				>
					<X />
				</Button>
				<DrawerHeader className="pr-12">
					<DrawerTitle>{name}</DrawerTitle>
					<DrawerDescription>{namespace} · Pod YAML</DrawerDescription>
				</DrawerHeader>
				{/* A right drawer spans the full height, so the editor fills the rest. */}
				<div className="min-h-0 flex-1 overflow-hidden border-t">
					{loading && (
						<p className="flex h-full items-center justify-center gap-2 text-muted-foreground">
							<LoaderCircle className="size-4 animate-spin" />
							Loading YAML…
						</p>
					)}
					{!loading && error !== "" && (
						<p className="h-full overflow-auto p-6 text-center text-red-400">
							{error}
						</p>
					)}
					{!loading && error === "" && <YamlViewer document={document} />}
				</div>
			</DrawerContent>
		</Drawer>
	);
}
