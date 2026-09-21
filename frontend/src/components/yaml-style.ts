import { HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { EditorView } from "codemirror";

// The YAML surfaces are monochrome: keys stay white while values step down to grey, so they
// match the app instead of pulling in CodeMirror's default colour palette. The read-only viewer
// and the editor share these definitions so the two never drift apart.
// basicSetup registers its own highlight style as a fallback, so this one takes precedence.
export const yamlHighlightStyle = HighlightStyle.define([
	{ tag: tags.propertyName, color: "var(--foreground)", fontWeight: "600" },
	{
		tag: [tags.string, tags.special(tags.string)],
		color: "var(--muted-foreground)",
	},
	{
		tag: [tags.number, tags.bool, tags.null, tags.atom],
		color: "var(--muted-foreground)",
	},
	{ tag: tags.comment, color: "var(--muted-foreground)", fontStyle: "italic" },
	{
		tag: [tags.punctuation, tags.separator, tags.bracket],
		color: "var(--muted-foreground)",
	},
]);

export const yamlTheme = EditorView.theme(
	{
		"&": {
			height: "100%",
			backgroundColor: "transparent",
			color: "var(--foreground)",
			fontSize: "13px",
		},
		".cm-scroller": {
			fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
			lineHeight: "1.6",
		},
		".cm-gutters": {
			backgroundColor: "transparent",
			border: "none",
			color: "var(--muted-foreground)",
		},
		".cm-activeLine": {
			backgroundColor: "color-mix(in oklab, var(--foreground) 6%, transparent)",
		},
		".cm-activeLineGutter": {
			backgroundColor: "transparent",
			color: "var(--foreground)",
		},
		".cm-selectionBackground, ::selection": {
			backgroundColor:
				"color-mix(in oklab, var(--foreground) 22%, transparent)",
		},
	},
	{ dark: true },
);
