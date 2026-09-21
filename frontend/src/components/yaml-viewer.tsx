import { yaml } from "@codemirror/lang-yaml";
import { syntaxHighlighting } from "@codemirror/language";
import { cn } from "cn";
import { basicSetup, EditorView } from "codemirror";
import { useEffect, useRef } from "react";
import { yamlHighlightStyle, yamlTheme } from "@/components/yaml-style";

// The viewer is the read-only twin of the editor: the same monochrome look from yaml-style.ts,
// with editing turned off.
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
