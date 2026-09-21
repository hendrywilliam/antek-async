import { yaml } from "@codemirror/lang-yaml";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { cn } from "cn";
import { basicSetup, EditorView } from "codemirror";
import { useEffect, useRef } from "react";

// The YAML viewer is read-only and monochrome: keys stay white while values step down to grey,
// so it matches the app instead of pulling in CodeMirror's default colour palette.
// basicSetup registers its own highlight style as a fallback, so this one takes precedence.
const yamlHighlightStyle = HighlightStyle.define([
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

const yamlTheme = EditorView.theme(
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

const yamlExtensions = [
	basicSetup,
	yaml(),
	syntaxHighlighting(yamlHighlightStyle),
	EditorView.editable.of(false),
	yamlTheme,
];

// YamlViewer mounts CodeMirror into a plain div and tears it down with the document, which is
// cheap because it only ever shows one resource at a time. It is read-only and monochrome.
export function YamlViewer({
	document,
	className,
}: {
	document: string;
	className?: string;
}) {
	const container = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const parent = container.current;
		if (parent == null) {
			return;
		}

		const view = new EditorView({
			doc: document,
			extensions: yamlExtensions,
			parent,
		});

		return () => view.destroy();
	}, [document]);

	return (
		<div className={cn("h-full overflow-hidden", className)} ref={container} />
	);
}
