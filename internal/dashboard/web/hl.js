// Progressive highlighting of fenced code blocks. md.js renders blocks plain
// (textContent, never innerHTML) with a data-lang, then loads this module
// once via dynamic import and calls enhance(root) on every render, including
// the editor Preview. Unknown languages and blocks over MAX stay plain, so a
// failed import or an old browser only loses colors, never content.
//
// Vendored speed-highlight core 2.1.0 (CC0-1.0,
// https://github.com/speed-highlight/core): tokenizeWith is synchronous and
// never touches the DOM; spans below are built with textContent. Grammars
// live in ./shj/; a new language is one grammar file plus one line in
// GRAMMARS (kotlin.js is ours, written in the same rule format).
import { tokenizeWith } from "./shj/tokenize.js";
import go from "./shj/go.js";
import js from "./shj/js.js";
import ts from "./shj/ts.js";
import bash from "./shj/bash.js";
import py from "./shj/py.js";
import json from "./shj/json.js";
import yaml from "./shj/yaml.js";
import java from "./shj/java.js";
import diff from "./shj/diff.js";
import kotlin from "./shj/kotlin.js";
// Sub-grammars other grammars delegate to (comments, regexes, templates).
import jsdoc from "./shj/jsdoc.js";
import todo from "./shj/todo.js";
import regex from "./shj/regex.js";

const GRAMMARS = { go, js, ts, bash, py, json, yaml, java, diff, kotlin, jsdoc, todo, regex };
const ALIAS = { javascript: "js", jsx: "js", typescript: "ts", tsx: "ts",
  shell: "bash", zsh: "bash", sh: "bash", python: "py", yml: "yaml",
  kt: "kotlin", kts: "kotlin" };
for (const k in GRAMMARS) ALIAS[k] = k;

// Token types outside CLASS render as plain text (operators, escapes).
const CLASS = { kwd: "tok-k", str: "tok-s", cmnt: "tok-c", num: "tok-n",
  bool: "tok-n", var: "tok-n", func: "tok-f", class: "tok-f", type: "tok-k",
  insert: "tok-i", deleted: "tok-d", section: "tok-k", err: "tok-d" };

const MAX = 20000;

export function enhance(root) {
  if (!root || !root.querySelectorAll) return;
  root.querySelectorAll("code[data-lang]").forEach((code) => {
    const src = code.textContent;
    const g = GRAMMARS[ALIAS[String(code.getAttribute("data-lang") || "").toLowerCase()]];
    if (!g || !src || src.length > MAX || !code.isConnected) return;
    const frag = document.createDocumentFragment();
    try {
      tokenizeWith(src, g, (text, type) => {
        if (!text) return;
        const cls = CLASS[type];
        if (!cls) {
          frag.appendChild(document.createTextNode(text));
          return;
        }
        const s = document.createElement("span");
        s.className = cls;
        s.textContent = text;
        frag.appendChild(s);
      }, { languages: GRAMMARS });
    } catch (e) {
      return;
    }
    if (!code.isConnected || code.textContent !== src) return; // stale render
    code.textContent = "";
    code.appendChild(frag);
  });
}
