import { yaml } from "@codemirror/lang-yaml";
import { syntaxHighlighting } from "@codemirror/language";
import { cn } from "cn";
import { basicSetup, EditorView } from "codemirror";
import { useEffect, useRef } from "react";
import { yamlHighlightStyle, yamlTheme } from "@/components/yaml-style";

// YamlEditor mounts CodeMirror in edit mode and reports the document on every change. The view
// is built once per seed document: rebuilding it on each keystroke would reset the cursor and
// the undo history, so the live value is never handed back in as `initialDocument`. To empty the
// editor, remount it with a new `key` instead of changing this prop.
export function YamlEditor({
	initialDocument,
	onChange,
	className,
}: {
	initialDocument: string;
	onChange: (document: string) => void;
	className?: string;
}) {
	const container = useRef<HTMLDivElement>(null);

	// The listener is registered once with the view, so the newest handler is reached through a
	// ref instead of rebuilding the view whenever the parent re-renders.
	const onChangeRef = useRef(onChange);
	useEffect(() => {
		onChangeRef.current = onChange;
	}, [onChange]);

	useEffect(() => {
		const parent = container.current;
		if (parent == null) {
			return;
		}

		const view = new EditorView({
			doc: initialDocument,
			extensions: [
				basicSetup,
				yaml(),
				syntaxHighlighting(yamlHighlightStyle),
				yamlTheme,
				EditorView.updateListener.of((update) => {
					if (update.docChanged) {
						onChangeRef.current(update.state.doc.toString());
					}
				}),
			],
			parent,
		});

		return () => view.destroy();
		// initialDocument is a seed, not the live value: see the component comment above.
	}, [initialDocument]);

	return (
		<div className={cn("h-full overflow-hidden", className)} ref={container} />
	);
}
