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

// xterm has no clipboard of its own: on a right click it only moves the selection into the
// hidden textarea and leaves the copy to the browser's context menu, which a Wails webview does
// not show. Right click is therefore wired by hand, and it is a session's only copy gesture.
// Pass the terminal's host, not its screen element: the capture pass on an ancestor runs before
// xterm's own listeners, which is what keeps the selection readable at press time.
export function attachSelectionCopy(term: Terminal, element: HTMLElement): () => void {
  // The press is what copies, because the menu that would normally follow is not something a
  // webview on Linux delivers at all (which is also why xterm handles a right click on the press
  // there). The flag only keeps a menu that does arrive from copying the same text twice.
  let copiedOnPress = false;

  const onMouseDown = (event: MouseEvent) => {
    if (event.button !== 2) {
      return;
    }
    copiedOnPress = false;
    const selection = term.getSelection();
    if (selection === "") {
      return;
    }
    copiedOnPress = true;
    copyText(selection);
  };

  const onContextMenu = (event: MouseEvent) => {
    const selection = term.getSelection();
    if (selection === "") {
      // Nothing to copy, so a right click goes on doing whatever it did before.
      return;
    }
    event.preventDefault();
    if (copiedOnPress) {
      copiedOnPress = false;
      return;
    }
    // A menu opened without a press, the keyboard's menu key for instance.
    copyText(selection);
  };

  element.addEventListener("mousedown", onMouseDown, true);
  element.addEventListener("contextmenu", onContextMenu, true);

  return () => {
    element.removeEventListener("mousedown", onMouseDown, true);
    element.removeEventListener("contextmenu", onContextMenu, true);
  };
}

function copyText(text: string) {
  // A wails:// page is not always a secure context, which the async clipboard API requires, so
  // the old execCommand path stays as the fallback rather than leaving the copy silently lost.
  if (typeof navigator.clipboard?.writeText === "function") {
    navigator.clipboard.writeText(text).catch(() => legacyCopy(text));
    return;
  }
  legacyCopy(text);
}

function legacyCopy(text: string) {
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "0";
  area.style.left = "0";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  try {
    document.execCommand("copy");
  } catch {
    // Nothing left to try; the selection is still on screen for a manual copy.
  }
  document.body.removeChild(area);
}
