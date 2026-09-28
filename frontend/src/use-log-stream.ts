import { useCallback, useEffect, useState } from "react";
import { OpenPodLogs } from "../wailsjs/go/main/App";
import { kube } from "../wailsjs/go/models";
import { attachSelectionCopy, createTerminal } from "@/terminal-theme";
import { parseControl, type TerminalStatus } from "@/use-terminal";

// How much history the viewer asks for before it starts following. A pod's log can be long and the
// terminal's scrollback is bounded anyway, so a bounded tail is what makes opening a chatty pod
// useful instead of slow.
const LOG_TAIL_LINES = 200;

export type LogTarget = {
	namespace: string;
	pod: string;
};

export type LogSession = {
	status: TerminalStatus;
	error: string;
	exitCode: number | null;
	// container is the one the cluster streamed, which is the pod's first container for now.
	container: string;
	reconnect: () => void;
};

// useLogStream owns one followed log, from the xterm instance to the WebSocket behind it. It is
// the read-only sibling of `useTerminal`: the stream travels one way, so no typed input is sent
// and no size is reported, and the server resolves which container to read. The session starts
// when `host` is a live element and `target` names a pod, and the effect's cleanup ends it, which
// means closing the drawer is enough to stop the stream.
export function useLogStream(
	host: HTMLDivElement | null,
	target: LogTarget | null,
): LogSession {
	const [status, setStatus] = useState<TerminalStatus>("connecting");
	const [error, setError] = useState("");
	const [exitCode, setExitCode] = useState<number | null>(null);
	const [container, setContainer] = useState("");
	const [attempt, setAttempt] = useState(0);

	const reconnect = useCallback(() => setAttempt((current) => current + 1), []);

	const namespace = target?.namespace ?? "";
	const pod = target?.pod ?? "";

	useEffect(() => {
		if (host == null || namespace === "" || pod === "") {
			return;
		}

		let cancelled = false;
		let socket: WebSocket | null = null;

		// The previous session's terminal can still be in the element when the pod changes.
		host.replaceChildren();

		const { term, fit } = createTerminal("logs");
		term.open(host);
		const detachCopy = attachSelectionCopy(term, host);

		setStatus("connecting");
		setError("");
		setExitCode(null);
		setContainer("");

		// The drawer animates in, so the box can still be unmeasured here. Nothing is fitted in
		// that case, and the observer below runs again once the box has a size.
		const measure = () => {
			if (host.clientWidth === 0 || host.clientHeight === 0) {
				return;
			}
			try {
				fit.fit();
			} catch {
				// xterm refuses to fit into a box it cannot read; the next observation retries.
			}
		};
		measure();

		const observer = new ResizeObserver(measure);
		observer.observe(host);

		// The container is left empty so the server picks the pod's first one, which is what the
		// endpoint then reports back for the drawer to show.
		const request: kube.LogRequest = {
			namespace,
			pod,
			container: "",
			follow: true,
			tailLines: LOG_TAIL_LINES,
		};

		OpenPodLogs(request)
			.then((endpoint) => {
				if (cancelled) {
					// The drawer closed while the stream was being prepared. Nothing was read from
					// the cluster, and the endpoint is simply left to expire.
					return;
				}

				setContainer(endpoint.container);

				const opened = new WebSocket(endpoint.url);
				socket = opened;

				// Without this the browser hands back a Blob and every output chunk is lost.
				opened.binaryType = "arraybuffer";

				opened.onmessage = (event: MessageEvent<string | ArrayBuffer>) => {
					if (typeof event.data === "string") {
						const message = parseControl(event.data);
						if (message?.type === "ready") {
							setStatus("open");
						}
						if (message?.type === "exit") {
							setStatus("closed");
							setExitCode(message.code ?? 0);
							setError(message.reason ?? "");
						}
						return;
					}

					term.write(new Uint8Array(event.data));
				};

				opened.onclose = () => {
					if (cancelled) {
						return;
					}
					// A stream that already ended keeps the exit it reported.
					setStatus((current) =>
						current === "connecting" || current === "open" ? "closed" : current,
					);
				};

				opened.onerror = () => {
					if (!cancelled) {
						setStatus("error");
						setError(
							"The log connection could not be opened. Check that this window is allowed to reach the local terminal port.",
						);
					}
				};
			})
			.catch((err: unknown) => {
				if (!cancelled) {
					setStatus("error");
					setError(String(err));
				}
			});

		return () => {
			cancelled = true;
			observer.disconnect();
			detachCopy();
			// Closing the socket is what ends the stream on the other side.
			socket?.close();
			term.dispose();
		};
	}, [host, namespace, pod, attempt]);

	return { status, error, exitCode, container, reconnect };
}
