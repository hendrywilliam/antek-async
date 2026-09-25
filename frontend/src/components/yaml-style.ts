import { HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { EditorView } from "codemirror";

export const yamlHighlightStyle = HighlightStyle.define([
  {
    tag: tags.propertyName,
    color: "#61AFEF", // key/property → biru
    fontWeight: "600",
  },
  {
    tag: [tags.string, tags.special(tags.string), tags.content],
    color: "#98C379", // string value → hijau
  },
  {
    tag: [tags.number, tags.bool, tags.null, tags.atom],
    color: "#D19A66", // number/boolean/null → oranye
  },
  {
    tag: [tags.labelName, tags.typeName, tags.keyword],
    color: "#C678DD", // keyword/anchor/tag → ungu
  },
  {
    tag: tags.meta,
    color: "#56B6C2", // meta (mis. document markers ---) → cyan
  },
  {
    tag: [tags.comment, tags.lineComment],
    color: "var(--muted-foreground)",
    fontStyle: "italic",
  },
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
