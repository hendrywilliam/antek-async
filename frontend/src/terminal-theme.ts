import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";

export const TERMINAL_THEME = {
  background: "#000000",
  foreground: "#f4f4f5",
  cursor: "#ffffff",
  cursorAccent: "#000000",
  selectionBackground: "#3f3f46",
  black: "#27272a",
  red: "#f87171",
  green: "#4ade80",
  yellow: "#facc15",
  blue: "#60a5fa",
  magenta: "#e879f9",
  cyan: "#22d3ee",
  white: "#e4e4e7",
  brightBlack: "#71717a",
  brightRed: "#fca5a5",
  brightGreen: "#86efac",
  brightYellow: "#fde047",
  brightBlue: "#93c5fd",
  brightMagenta: "#f0abfc",
  brightCyan: "#67e8f9",
  brightWhite: "#fafafa",
} as const;

export const TERMINAL_FONT =
  '"JetBrains Mono Variable", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

export type SessionKind = "terminal" | "logs";

export function createTerminal(kind: SessionKind): {
  term: Terminal;
  fit: FitAddon;
} {
  const logs = kind === "logs";

  const term = new Terminal({
    convertEol: logs,
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
