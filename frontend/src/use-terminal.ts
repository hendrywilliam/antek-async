import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import { useCallback, useEffect, useState } from "react";
import { OpenTerminal } from "../wailsjs/go/main/App";
import { kube } from "../wailsjs/go/models";

// The app is black, white and grey, and the terminal keeps that rule rather than letting a shell
// paint its own colours into the window. The ANSI entries are therefore a grey ramp; the literals
// are used instead of the oklch() tokens in style.css because xterm's colour parser cannot read
// those.
const TERMINAL_THEME = {
	background: "#000000",
	foreground: "#ffffff",
	cursor: "#ffffff",
	cursorAccent: "#000000",
	selectionBackground: "#3f3f46",
	black: "#27272a",
	red: "#a1a1aa",
	green: "#d4d4d8",
	yellow: "#e4e4e7",
	blue: "#d4d4d8",
	magenta: "#a1a1aa",
	cyan: "#d4d4d8",
	white: "#e4e4e7",
	brightBlack: "#71717a",
	brightRed: "#d4d4d8",
	brightGreen: "#e4e4e7",
	brightYellow: "#f4f4f5",
	brightBlue: "#e4e4e7",
	brightMagenta: "#d4d4d8",
	brightCyan: "#e4e4e7",
	brightWhite: "#ffffff",
};

// The mono font is bundled with the app the same way Inter is, so a terminal never depends on
// what the machine happens to have installed.
const TERMINAL_FONT =
	'"JetBrains Mono Variable", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

// A terminal that has not been measured yet still has to name a size, because the API server
// allocates the PTY before the socket can report a resize.
const FALLBACK_COLS = 80;
const FALLBACK_ROWS = 24;

// Output never arrives as text, so a text frame is always one of these two control messages.
type TerminalControl = {
	type: string;
	code?: number;
	reason?: string;
};

export type TerminalStatus = "connecting" | "open" | "closed" | "error";

export type TerminalSession = {
	status: TerminalStatus;
	error: string;
	exitCode: number | null;
	reconnect: () => void;
};

export type TerminalTarget = {
	namespace: string;
	pod: string;
	container: string;
};

function parseControl(payload: string): TerminalControl | null {
	try {
		return JSON.parse(payload) as TerminalControl;
	} catch {
		return null;
	}
}

// useTerminal owns one interactive session, from the xterm instance to the WebSocket behind it.
// The drawer only says which pod and container to show, so all of the protocol lives here. The
// session starts when `host` is a live element and `target` names a container, and the effect's
// cleanup ends it, which means unmounting the drawer is enough to stop everything.
export function useTerminal(
	host: HTMLDivElement | null,
	target: TerminalTarget | null,
): TerminalSession {
	const [status, setStatus] = useState<TerminalStatus>("connecting");
	const [error, setError] = useState("");
	const [exitCode, setExitCode] = useState<number | null>(null);
	const [attempt, setAttempt] = useState(0);

	const reconnect = useCallback(() => setAttempt((current) => current + 1), []);

	const namespace = target?.namespace ?? "";
	const pod = target?.pod ?? "";
	const container = target?.container ?? "";

	useEffect(() => {
		if (host == null || namespace === "" || pod === "" || container === "") {
			return;
		}

		let cancelled = false;
		let socket: WebSocket | null = null;

		// The previous session's terminal can still be in the element when the container changes.
		host.replaceChildren();

		const term = new Terminal({
			cursorBlink: true,
			fontFamily: TERMINAL_FONT,
			fontSize: 13,
			scrollback: 5000,
			theme: TERMINAL_THEME,
		});
		const fit = new FitAddon();
		term.loadAddon(fit);
		term.open(host);

		setStatus("connecting");
		setError("");
		setExitCode(null);

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

		const send = (message: object) => {
			if (socket?.readyState === WebSocket.OPEN) {
				socket.send(JSON.stringify(message));
			}
		};

		// Typed input goes out as bytes, the same way output arrives, so a pasted multi-byte
		// character is not at the mercy of a frame boundary.
		term.onData((data) => {
			if (socket?.readyState === WebSocket.OPEN) {
				socket.send(new TextEncoder().encode(data));
			}
		});

		term.onResize(({ cols, rows }) => send({ type: "resize", cols, rows }));

		const request: kube.TerminalRequest = {
			namespace,
			pod,
			container,
			command: [],
			cols: term.cols > 0 ? term.cols : FALLBACK_COLS,
			rows: term.rows > 0 ? term.rows : FALLBACK_ROWS,
		};

		OpenTerminal(request)
			.then((endpoint) => {
				if (cancelled) {
					// The drawer closed while the session was being prepared. Nothing was opened
					// against the cluster, and the endpoint is simply left to expire.
					return;
				}

				const opened = new WebSocket(endpoint.url);
				socket = opened;

				// Without this the browser hands back a Blob and every output chunk is lost.
				opened.binaryType = "arraybuffer";

				opened.onopen = () => {
					// The size the session started with may have been measured before the drawer
					// settled, so it is sent again now that the box has its final size.
					send({ type: "resize", cols: term.cols, rows: term.rows });
				};

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
					// A session that already ended keeps the exit it reported.
					setStatus((current) =>
						current === "connecting" || current === "open" ? "closed" : current,
					);
				};

				opened.onerror = () => {
					if (!cancelled) {
						setStatus("error");
						setError(
							"The terminal connection could not be opened. Check that this window is allowed to reach the local terminal port.",
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
			// Closing the socket is what ends the session on the other side, so it happens before
			// the terminal is thrown away.
			socket?.close();
			term.dispose();
		};
	}, [host, namespace, pod, container, attempt]);

	return { status, error, exitCode, reconnect };
}
