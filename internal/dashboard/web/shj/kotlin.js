// Kotlin grammar for speed-highlight core 2.1.0 (rule format: earliest match
// wins; see https://speed-highlight.github.io/core/#custom-languages).
// Written for baton (no upstream Kotlin grammar); public domain (CC0-1.0)
// like the rest of @speed-highlight/core.
var t = [
  { match: /\/\/.*\n?|\/\*((?!\*\/)[^])*(\*\/)?/g, type: "cmnt", sub: "todo" },
  { type: "str", match: /"""(\\[^]|(?!""")[^])*(""")?/g },
  { expand: "str" },
  { expand: "num" },
  { type: "kwd", match: /\b(as|break|class|continue|do|else|false|for|fun|if|in|interface|is|null|object|package|return|super|this|throw|true|try|val|var|when|while|by|catch|constructor|delegate|field|finally|get|import|init|out|set|where|abstract|actual|annotation|companion|const|crossinline|data|enum|expect|external|final|infix|inline|inner|internal|lateinit|open|operator|override|private|protected|public|reified|sealed|tailrec|vararg|suspend)\b/g },
  { type: "func", match: /[a-zA-Z_][\w_]*(?=\s*\()/g },
  { type: "class", match: /\b[A-Z][\w_]*\b/g },
];
export { t as default };
