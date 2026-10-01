"use strict";
// Minimal Markdown renderer that builds DOM nodes (never innerHTML), so
// handoff text cannot inject markup.
window.renderMarkdown = (function () {
  function el(tag, cls, text) {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text !== undefined) n.textContent = text;
    return n;
  }

  const inlineRe = /(`+)([\s\S]*?)\1|\*\*((?:[^*]|\*(?!\*))+)\*\*|__([^_]+)__|\*([^*\s][^*]*)\*|\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])/g;

  function inline(parent, text) {
    // matchAll iterates a copy of the regex: the recursion for bold/em
    // would otherwise move a shared lastIndex and loop forever.
    let last = 0;
    for (const m of text.matchAll(inlineRe)) {
      if (m.index > last) parent.appendChild(document.createTextNode(text.slice(last, m.index)));
      if (m[1]) parent.appendChild(el("code", null, m[2]));
      else if (m[3] || m[4]) {
        const b = el("strong");
        inline(b, m[3] || m[4]);
        parent.appendChild(b);
      } else if (m[5]) {
        const i = el("em");
        inline(i, m[5]);
        parent.appendChild(i);
      } else if (m[6]) parent.appendChild(link(m[6], m[7]));
      else if (m[8]) parent.appendChild(link(m[8], m[8]));
      last = m.index + m[0].length;
    }
    if (last < text.length) parent.appendChild(document.createTextNode(text.slice(last)));
  }

  function link(text, href) {
    if (!/^https?:\/\//i.test(href)) return document.createTextNode(text);
    const a = el("a", null, text);
    a.href = href;
    a.rel = "noopener noreferrer";
    a.target = "_blank";
    return a;
  }

  return function render(src) {
    const root = el("div", "md");
    const lines = String(src || "").replace(/\r\n/g, "\n").split("\n");
    let i = 0;
    let para = [];
    const flush = () => {
      if (para.length) {
        const p = el("p");
        inline(p, para.join(" "));
        root.appendChild(p);
        para = [];
      }
    };
    while (i < lines.length) {
      const line = lines[i];
      let m;
      if ((m = /^\s*(```+|~~~+)\s*(\S*)/.exec(line))) {
        flush();
        const fence = m[1];
        const body = [];
        i++;
        while (i < lines.length && !lines[i].trim().startsWith(fence)) body.push(lines[i++]);
        i++;
        const pre = el("pre");
        pre.appendChild(el("code", m[2] ? "lang-" + m[2].replace(/[^\w-]/g, "") : null, body.join("\n")));
        root.appendChild(pre);
        continue;
      }
      if ((m = /^(#{1,6})\s+(.*)$/.exec(line))) {
        flush();
        const h = el("h" + Math.min(m[1].length + 1, 6));
        inline(h, m[2]);
        root.appendChild(h);
        i++;
        continue;
      }
      if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) {
        flush();
        root.appendChild(el("hr"));
        i++;
        continue;
      }
      if (/^\s*>/.test(line)) {
        flush();
        const q = [];
        while (i < lines.length && /^\s*>/.test(lines[i])) q.push(lines[i++].replace(/^\s*>\s?/, ""));
        const bq = render(q.join("\n"));
        const b = el("blockquote");
        while (bq.firstChild) b.appendChild(bq.firstChild);
        root.appendChild(b);
        continue;
      }
      if ((m = /^(\s*)([-*+]|\d+[.)])\s+/.exec(line))) {
        flush();
        const ordered = /\d/.test(m[2]);
        const list = el(ordered ? "ol" : "ul");
        while (i < lines.length && (m = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/.exec(lines[i]))) {
          let text = m[3];
          i++;
          while (i < lines.length && /^\s{2,}\S/.test(lines[i]) && !/^\s*([-*+]|\d+[.)])\s+/.test(lines[i])) {
            text += " " + lines[i++].trim();
          }
          const li = el("li");
          const task = /^\[([ xX])\]\s+(.*)$/.exec(text);
          if (task) {
            const cb = el("input");
            cb.type = "checkbox";
            cb.disabled = true;
            cb.checked = task[1] !== " ";
            li.appendChild(cb);
            li.appendChild(document.createTextNode(" "));
            text = task[2];
          }
          inline(li, text);
          list.appendChild(li);
        }
        root.appendChild(list);
        continue;
      }
      if (line.trim() === "") {
        flush();
        i++;
        continue;
      }
      para.push(line.trim());
      i++;
    }
    flush();
    return root;
  };
})();
