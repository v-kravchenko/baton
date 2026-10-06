"use strict";
// Minimal Markdown renderer that builds DOM nodes (never innerHTML), so
// handoff text cannot inject markup. With opts.onCheck(line, checked, box),
// "[ ]" items are clickable; line is the item's 0-based line in src. With
// opts.ref(name, label), [[name]], [[name|label]] and @name may become links:
// it returns a node, or null to keep the text.
window.renderMarkdown = (function () {
  function el(tag, cls, text) {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text !== undefined) n.textContent = text;
    return n;
  }

  const inlineRe = /(`+)([\s\S]*?)\1|\*\*((?:[^*]|\*(?!\*))+)\*\*|__([^_]+)__|~~([^~]+?)~~|\*([^*\s][^*]*)\*|!\[([^\]]*)\]\(([^)\s]+)\)|\[\[([^\]|]+)(?:\|([^\]]+))?\]\]|\[([^\]]+)\]\(([^)\s]+)\)|(https?:\/\/[^\s<>()]+[^\s<>().,;:!?'"])|(?<![\w@./-])@([a-z0-9](?:[a-z0-9._-]*[a-z0-9])?)/g;
  let opts = {}; // of the render call in progress (inline reads ref)

  // Code highlighting is progressive: blocks render plain (textContent, never
  // innerHTML) with a data-lang, then hl.js (vendored speed-highlight) colors
  // them in place. A failed import only loses colors, never content.
  let hlP = null;
  function hlLater(root) {
    try {
      hlP = hlP || import("/static/hl.js");
      hlP.then((m) => { try { m.enhance(root); } catch (e) { /* plain */ } }, () => {});
    } catch (e) { /* old browser: stay plain */ }
  }

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
        const d = el("del");
        inline(d, m[5]);
        parent.appendChild(d);
      } else if (m[6]) {
        const i = el("em");
        inline(i, m[6]);
        parent.appendChild(i);
      } else if (m[7] !== undefined) parent.appendChild(img(m[7], m[8]));
      else if (m[9]) {
        const name = m[9].trim(), label = (m[10] || m[9]).trim();
        parent.appendChild((opts.ref && opts.ref(name, label)) || document.createTextNode(label));
      } else if (m[11]) parent.appendChild(link(m[11], m[12]));
      else if (m[13]) parent.appendChild(link(m[13], m[13]));
      else if (m[14]) parent.appendChild((opts.ref && opts.ref(m[14], "@" + m[14])) || document.createTextNode(m[0]));
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

  // Images stay in the never-innerHTML model (src/alt set as properties).
  // data: and relative paths need no CSP change; http(s) is covered by
  // img-src in server.go. Anything else (e.g. javascript:) falls back to a link.
  function img(alt, src) {
    if (/^data:image\//i.test(src) || /^https?:\/\//i.test(src) || !/^[a-zA-Z][\w+.-]*:/.test(src)) {
      const im = document.createElement("img");
      im.alt = alt;
      im.src = src;
      im.loading = "lazy";
      return im;
    }
    return link(alt || src, src);
  }

  // GFM table row: one leading/trailing pipe stripped, \| stays literal.
  function splitRow(line) {
    let s = line.trim();
    if (s.startsWith("|")) s = s.slice(1);
    if (s.endsWith("|")) s = s.slice(0, -1);
    const cells = [];
    let cur = "";
    for (let k = 0; k < s.length; k++) {
      const c = s[k];
      if (c === "\\" && k + 1 < s.length && (s[k + 1] === "|" || s[k + 1] === "\\")) cur += s[++k];
      else if (c === "|") { cells.push(cur.trim()); cur = ""; }
      else cur += c;
    }
    cells.push(cur.trim());
    return cells;
  }
  const isDelimCell = (c) => /^:?-+:?$/.test(c.trim());

  function render(src, o) {
    const outer = opts;
    opts = o || {};
    try {
      const root = blocks(src);
      hlLater(root);
      return root;
    } finally { opts = outer; }
  }

  function blocks(src) {
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
        const info = (m[2] || "").replace(/[^\w-]/g, "");
        const code = el("code", info ? "lang-" + info : null);
        if (info) code.setAttribute("data-lang", info);
        code.textContent = body.join("\n");
        pre.appendChild(code);
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
        const bq = blocks(q.join("\n"));
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
          const at = i;
          i++;
          while (i < lines.length && /^\s{2,}\S/.test(lines[i]) && !/^\s*([-*+]|\d+[.)])\s+/.test(lines[i])) {
            text += " " + lines[i++].trim();
          }
          const li = el("li");
          const task = /^\[([ xX])\]\s+(.*)$/.exec(text);
          if (task) {
            const cb = el("input");
            cb.type = "checkbox";
            cb.checked = task[1] !== " ";
            if (opts.onCheck) {
              li.className = "check";
              cb.addEventListener("change", () => opts.onCheck(at, cb.checked, cb));
            } else cb.disabled = true;
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
      if (line.includes("|") && i + 1 < lines.length) {
        const head = splitRow(line);
        const delim = splitRow(lines[i + 1]);
        if (delim.length === head.length && delim.every(isDelimCell)) {
          flush();
          const aligns = delim.map((d) => {
            d = d.trim();
            const l = d.startsWith(":"), r = d.endsWith(":");
            return l && r ? "center" : l ? "left" : r ? "right" : "";
          });
          const table = el("table");
          const thead = el("thead");
          const htr = el("tr");
          head.forEach((c, ci) => {
            const th = el("th");
            if (aligns[ci]) th.style.textAlign = aligns[ci]; // CSSOM: not blocked by CSP style-src
            inline(th, c);
            htr.appendChild(th);
          });
          thead.appendChild(htr);
          table.appendChild(thead);
          const tb = el("tbody");
          i += 2;
          while (i < lines.length && lines[i].trim() !== "" && lines[i].includes("|")) {
            const cells = splitRow(lines[i]);
            const tr = el("tr");
            head.forEach((_, ci) => {
              const td = el("td");
              if (aligns[ci]) td.style.textAlign = aligns[ci];
              inline(td, cells[ci] || "");
              tr.appendChild(td);
            });
            tb.appendChild(tr);
            i++;
          }
          table.appendChild(tb);
          root.appendChild(table);
          continue;
        }
      }
      para.push(line.trim());
      i++;
    }
    flush();
    return root;
  }
  return render;
})();
