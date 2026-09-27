import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";

// The app is black, white and grey, and the terminal keeps that rule rather than letting a shell
// paint its own colours into the window. The ANSI entries are therefore a grey ramp; the literals
// are used instead of the oklch() tokens in style.css because xterm's colour parser cannot read
// those.
export const TERMINAL_THEME = {
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
export const TERMINAL_FONT =
	'"JetBrains Mono Variable", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

// SessionKind names the two pod sessions a terminal is built for. It mirrors the backend's session
// kinds, and the difference between them is what a PTY does: an interactive shell sits behind one,
// a followed log does not.
export type SessionKind = "terminal" | "logs";

// createTerminal builds the emulator both the interactive terminal and the log viewer use, so the
// two never drift apart.
export function createTerminal(kind: SessionKind): {
	term: Terminal;
	fit: FitAddon;
} {
	const logs = kind === "logs";

	const term = new Terminal({
		// A log line ends in a bare LF and there is no PTY behind it to turn that into a CRLF, so
		// xterm would only move the caret down a line and leave it in the same column, which paints
		// the whole log as a staircase. A shell behind a PTY already ends its lines with CR, so it
		// needs nothing here.
		convertEol: logs,
		// Nothing can be typed into a followed log, so it takes no input and has no blinking caret.
		cursorBlink: !logs,
		disableStdin: logs,
		fontFamily: TERMINAL_FONT,
		fontSize: 13,
		scrollback: 5000,
		theme: TERMINAL_THEME,
	});

	const fit = new FitAddon();
	term.loadAddon(fit);

	return { term, fit };
}
