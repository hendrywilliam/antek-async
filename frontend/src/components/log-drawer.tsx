import { LoaderCircle, X } from "lucide-react";
import { useState } from "react";
import { useLogStream } from "@/use-log-stream";
import { useDrawerResize } from "@/use-drawer-resize";
import { DrawerResizeHandle } from "@/components/drawer-resize-handle";
import { Button } from "@/components/ui/button";
import {
	Drawer,
	DrawerContent,
	DrawerDescription,
	DrawerHeader,
	DrawerTitle,
} from "@/components/ui/drawer";

// LogDrawer streams one pod's log into a read-only terminal. It is the read-only sibling of
// TerminalDrawer: nothing is typed back, and there is no container picker because the cluster
// streams the pod's first container for now. Pass a null `target` to keep it closed and call
// `onClose` to dismiss it.
export function LogDrawer({
	target,
	onClose,
}: {
	target: { namespace: string; name: string } | null;
	onClose: () => void;
}) {
	const namespace = target?.namespace ?? "";
	const name = target?.name ?? "";

	// The host is state rather than a ref so the hook re-runs when the element appears, which it
	// does only after the drawer's content is on screen.
	const [host, setHost] = useState<HTMLDivElement | null>(null);

	// The drawer can be dragged wider or narrower, and the log refits to it through the same
	// ResizeObserver that fits the first paint.
	const resize = useDrawerResize();

	const session = useLogStream(
		host,
		namespace === "" || name === "" ? null : { namespace, pod: name },
	);

	const failed = session.status === "error" && session.error !== "";
	const ended = session.status === "closed" && !failed;

	const exitLine = () => {
		const reason = session.error.trim();
		if (reason !== "") {
			return reason;
		}
		if (session.exitCode !== null && session.exitCode !== 0) {
			return `Log stream ended with exit code ${session.exitCode}`;
		}
		return "Log stream ended";
	};

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
			<DrawerContent style={resize.style}>
				<DrawerResizeHandle {...resize.handleProps} />
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
					<DrawerDescription>
						{session.container === ""
							? `${namespace} · Logs`
							: `${namespace} · ${session.container} · Logs`}
					</DrawerDescription>
				</DrawerHeader>
				{/* A right drawer spans the full height, so the log fills the rest, and the resize
				 * handle only ever changes its width. The host stays mounted behind the states below so a
				 * stream is never torn down just to show a message about it. */}
				<div className="relative max-h-full min-h-0 flex-1 overflow-hidden border-t">
					<div className="h-full w-full" data-slot="terminal-host" ref={setHost} />
					{session.status === "connecting" && !failed && (
						<p className="absolute inset-0 flex items-center justify-center gap-2 bg-background text-muted-foreground">
							<LoaderCircle className="size-4 animate-spin" />
							Connecting to logs…
						</p>
					)}
					{failed && (
						<div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background p-6">
							<p className="max-w-lg text-center break-words text-red-700">
								{session.error}
							</p>
							<Button onClick={session.reconnect} size="sm" variant="outline">
								Reconnect
							</Button>
						</div>
					)}
				</div>
				{ended && (
					<div className="flex items-center justify-between gap-3 border-t px-4 py-2 text-muted-foreground">
						<span className="min-w-0 break-words">{exitLine()}</span>
						<Button onClick={session.reconnect} size="sm" variant="outline">
							Reconnect
						</Button>
					</div>
				)}
			</DrawerContent>
		</Drawer>
	);
}
