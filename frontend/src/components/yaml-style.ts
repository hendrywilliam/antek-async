import { HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { EditorView } from "codemirror";

export const yamlHighlightStyle = HighlightStyle.define([
  {
    tag: tags.propertyName,
    color: "#0369a1", // key/property → biru
    fontWeight: "600",
  },
  {
    tag: [tags.string, tags.special(tags.string), tags.content],
    color: "#047857", // string value → hijau
  },
  {
    tag: [tags.number, tags.bool, tags.null, tags.atom],
    color: "#b45309", // number/boolean/null → oranye
  },
  {
    tag: [tags.labelName, tags.typeName, tags.keyword],
    color: "#7e22ce", // keyword/anchor/tag → ungu
  },
  {
    tag: tags.meta,
    color: "#0e7490", // meta (mis. document markers ---) → cyan
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
  { dark: false },
);
