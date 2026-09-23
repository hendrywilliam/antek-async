import { LoaderCircle, X } from "lucide-react";
import { useEffect, useState } from "react";
import { GetPodContainers } from "../../wailsjs/go/main/App";
import { useTerminal } from "@/use-terminal";
import { Button } from "@/components/ui/button";
import {
	Drawer,
	DrawerContent,
	DrawerDescription,
	DrawerHeader,
	DrawerTitle,
} from "@/components/ui/drawer";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";

// TerminalDrawer opens an interactive shell in one container of one pod, which makes it the only
// place in the app that streams in both directions: the session is a WebSocket this drawer owns,
// and it ends when the drawer closes. The terminal itself is a plain element the `useTerminal`
// hook fills, so nothing here touches xterm directly. Pass a null `target` to keep it closed and
// call `onClose` to dismiss it.
export function TerminalDrawer({
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
	const [containers, setContainers] = useState<string[]>([]);
	const [container, setContainer] = useState("");
	const [containersError, setContainersError] = useState("");
	const [attempt, setAttempt] = useState(0);

	useEffect(() => {
		if (namespace === "" || name === "") {
			return;
		}

		let cancelled = false;
		setContainers([]);
		setContainer("");
		setContainersError("");

		GetPodContainers(namespace, name)
			.then((result) => {
				if (cancelled) {
					return;
				}
				const names = result ?? [];
				setContainers(names);
				// The first container is what kubectl targets when no container is named.
				setContainer(names[0] ?? "");
			})
			.catch((err: unknown) => {
				if (!cancelled) {
					setContainersError(String(err));
				}
			});

		return () => {
			cancelled = true;
		};
	}, [namespace, name, attempt]);

	const session = useTerminal(
		host,
		namespace === "" || container === ""
			? null
			: { namespace, pod: name, container },
	);

	// A container list that could not be read and a session that could not be opened are both
	// failures of the drawer, so they share one treatment instead of two.
	const failed =
		containersError !== "" ||
		(session.status === "error" && session.error !== "");
	const ended = session.status === "closed" && !failed;

	const exitLine = () => {
		const reason = session.error.trim();
		if (reason !== "") {
			return reason;
		}
		if (session.exitCode !== null && session.exitCode !== 0) {
			return `Session ended with exit code ${session.exitCode}`;
		}
		return "Session ended";
	};

	const retry = () => {
		setAttempt((current) => current + 1);
		session.reconnect();
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
					<DrawerDescription>{namespace} · Terminal</DrawerDescription>
				</DrawerHeader>
				{containers.length > 0 && (
					<div className="flex items-center gap-3 border-t px-4 py-2">
						<span className="text-muted-foreground">Container</span>
						<Select onValueChange={setContainer} value={container}>
							<SelectTrigger className="w-64" size="sm">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{containers.map((item) => (
									<SelectItem key={item} value={item}>
										{item}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}
				{/* A right drawer spans the full height, so the terminal fills the rest. The host
				 * stays mounted behind the states below so a session is never torn down just to
				 * show a message about it. */}
				<div className="relative min-h-0 flex-1 overflow-hidden border-t">
					<div className="h-full w-full" ref={setHost} />
					{session.status === "connecting" && !failed && (
						<p className="absolute inset-0 flex items-center justify-center gap-2 bg-background text-muted-foreground">
							<LoaderCircle className="size-4 animate-spin" />
							Connecting to {container}…
						</p>
					)}
					{failed && (
						<div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background p-6">
							<p className="max-w-lg text-center break-words text-red-400">
								{containersError !== "" ? containersError : session.error}
							</p>
							<Button onClick={retry} size="sm" variant="outline">
								Reconnect
							</Button>
						</div>
					)}
				</div>
				{ended && (
					<div className="flex items-center justify-between gap-3 border-t px-4 py-2 text-muted-foreground">
						<span className="min-w-0 break-words">{exitLine()}</span>
						<Button onClick={retry} size="sm" variant="outline">
							Reconnect
						</Button>
					</div>
				)}
			</DrawerContent>
		</Drawer>
	);
}
