"use strict";
(function () {
  const $ = (s) => document.querySelector(s);
  const enc = encodeURIComponent;
  // open: key of the item in the panel; proj: the sidebar pick.
  const state = { q: "", archOpen: [], tab: {}, open: null, globalOpen: false, proj: "all" };
  let session = null;
  // data: { root, home, projects: [{ key, dir, updated, conflicts, tasks, tips }], global: [tips] }
  let data = null;
  let hits = new Map(); // full-text search: task key -> snippet
  let tipHits = new Set(); // full-text search: tip key
  const GLOBAL = "global";

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // Appends text with the search query wrapped in <mark>.
  function hl(parent, text) {
    const q = state.q.trim().toLowerCase();
    const s = String(text);
    if (q.length < 2) { parent.appendChild(document.createTextNode(s)); return parent; }
    const low = s.toLowerCase();
    let at = 0, i;
    while ((i = low.indexOf(q, at)) >= 0) {
      if (i > at) parent.appendChild(document.createTextNode(s.slice(at, i)));
      parent.appendChild(el("mark", "", s.slice(i, i + q.length)));
      at = i + q.length;
    }
    if (at < s.length) parent.appendChild(document.createTextNode(s.slice(at)));
    return parent;
  }

  // Text with `inline code` spans and search highlights; never parsed as HTML.
  function rich(tag, cls, text) {
    const e = el(tag, cls);
    String(text).split(/`([^`]+)`/).forEach((part, i) => {
      if (part) i % 2 ? e.appendChild(hl(el("code"), part)) : hl(e, part);
    });
    return e;
  }

  function age(iso) {
    // A bare date (tip source) has no time: count whole local days.
    const day = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso || "");
    if (day) {
      const dt = new Date(+day[1], day[2] - 1, +day[3]);
      dt.setFullYear(+day[1]);
      const d = Math.round((new Date().setHours(0, 0, 0, 0) - dt) / 86400000);
      return d <= 0 ? "today" : d === 1 ? "yesterday" : d < 14 ? d + "d ago" : dt.toLocaleDateString();
    }
    const t = Date.parse(iso);
    if (isNaN(t) || t <= 0) return "";
    const s = Math.max(0, (Date.now() - t) / 1000);
    if (s < 60) return "just now";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 86400) return Math.floor(s / 3600) + "h ago";
    if (s < 86400 * 14) return Math.floor(s / 86400) + "d ago";
    if (s < 86400 * 60) return Math.floor(s / 86400 / 7) + "w ago";
    return new Date(t).toLocaleDateString();
  }
  const when = (iso) => { const d = new Date(iso); return isNaN(d) ? "" : d.toLocaleString(); };

  function tilde(p) {
    const h = data && data.home;
    if (!p || !h || h === "/") return p;
    if (p === h) return "~";
    return p.startsWith(h + "/") || p.startsWith(h + "\\") ? "~" + p.slice(h.length) : p;
  }

  const ICONS = {
    // Lucide icons (ISC license, lucide.dev); circles, lines and rects written as paths.
    copy: ["M10 8h10a2 2 0 0 1 2 2v10a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2v-10a2 2 0 0 1 2 -2z", "M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"],
    check: ["M20 6 9 17l-5-5"],
    plus: ["M5 12h14", "M12 5v14"],
    fork: ["M9 18a3 3 0 1 0 6 0a3 3 0 1 0 -6 0", "M3 6a3 3 0 1 0 6 0a3 3 0 1 0 -6 0", "M15 6a3 3 0 1 0 6 0a3 3 0 1 0 -6 0",
      "M18 9v2c0 .6-.4 1-1 1H7c-.6 0-1-.4-1-1V9", "M12 12v3"],
    lightbulb: ["M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5", "M9 18h6", "M10 22h4"],
    ban: ["M2 12a10 10 0 1 0 20 0a10 10 0 1 0 -20 0", "m4.9 4.9 14.2 14.2"],
    type: ["M12 4v16", "M4 7V5a1 1 0 0 1 1-1h14a1 1 0 0 1 1 1v2", "M9 20h6"],
    undo: ["M9 14 4 9l5-5", "M4 9h10.5a5.5 5.5 0 0 1 5.5 5.5a5.5 5.5 0 0 1-5.5 5.5H11"],
    down: ["m6 9 6 6 6-6"],
    right: ["m9 18 6-6-6-6"],
    branch: ["M15 6a9 9 0 0 0-9 9V3", "M15.0 6a3 3 0 1 0 6.0 0a3 3 0 1 0 -6.0 0", "M3.0 18a3 3 0 1 0 6.0 0a3 3 0 1 0 -6.0 0"],
    copen: ["M2 12a10 10 0 1 0 20 0a10 10 0 1 0 -20 0", "M11 12a1 1 0 1 0 2 0a1 1 0 1 0 -2 0"],
    cdone: ["M2 12a10 10 0 1 0 20 0a10 10 0 1 0 -20 0", "m9 12 2 2 4-4"],
    commit: ["M9.0 12a3 3 0 1 0 6.0 0a3 3 0 1 0 -6.0 0", "M3 12L9 12", "M15 12L21 12"],
    close: ["M18 6 6 18", "m6 6 12 12"],
    logout: ["m16 17 5-5-5-5", "M21 12H9", "M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"],
    reload: ["M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8", "M21 3v5h-5"],
    edit: ["M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z", "m15 5 4 4"],
    trash: ["M10 11v6", "M14 11v6", "M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6", "M3 6h18", "M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"],
    github: ["M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4", "M9 18c-4.51 2-5-2-7-2"],
    gitlab: ["m22 13.29-3.33-10a.42.42 0 0 0-.14-.18.38.38 0 0 0-.22-.11.39.39 0 0 0-.23.07.42.42 0 0 0-.14.18l-2.26 6.67H8.32L6.1 3.26a.42.42 0 0 0-.1-.18.38.38 0 0 0-.26-.08.39.39 0 0 0-.23.07.42.42 0 0 0-.14.18L2 13.29a.74.74 0 0 0 .27.83L12 21l9.69-6.88a.71.71 0 0 0 .31-.83Z"],
    git: ["M6 3v12", "M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M15 6a9 9 0 0 0-9 9"],
    // Obsidian logo: Simple Icons 16.33.0 (CC0, simpleicons.org), drawn filled.
    obsidian: ["M19.355 18.538a68.967 68.959 0 0 0 1.858-2.954.81.81 0 0 0-.062-.9c-.516-.685-1.504-2.075-2.042-3.362-.553-1.321-.636-3.375-.64-4.377a1.707 1.707 0 0 0-.358-1.05l-3.198-4.064a3.744 3.744 0 0 1-.076.543c-.106.503-.307 1.004-.536 1.5-.134.29-.29.6-.446.914l-.31.626c-.516 1.068-.997 2.227-1.132 3.59-.124 1.26.046 2.73.815 4.481.128.011.257.025.386.044a6.363 6.363 0 0 1 3.326 1.505c.916.79 1.744 1.922 2.415 3.5zM8.199 22.569c.073.012.146.02.22.02.78.024 2.095.092 3.16.29.87.16 2.593.64 4.01 1.055 1.083.316 2.198-.548 2.355-1.664.114-.814.33-1.735.725-2.58l-.01.005c-.67-1.87-1.522-3.078-2.416-3.849a5.295 5.295 0 0 0-2.778-1.257c-1.54-.216-2.952.19-3.84.45.532 2.218.368 4.829-1.425 7.531zM5.533 9.938c-.023.1-.056.197-.098.29L2.82 16.059a1.602 1.602 0 0 0 .313 1.772l4.116 4.24c2.103-3.101 1.796-6.02.836-8.3-.728-1.73-1.832-3.081-2.55-3.831zM9.32 14.01c.615-.183 1.606-.465 2.745-.534-.683-1.725-.848-3.233-.716-4.577.154-1.552.7-2.847 1.235-3.95.113-.235.223-.454.328-.664.149-.297.288-.577.419-.86.217-.47.379-.885.46-1.27.08-.38.08-.72-.014-1.043-.095-.325-.297-.675-.68-1.06a1.6 1.6 0 0 0-1.475.36l-4.95 4.452a1.602 1.602 0 0 0-.513.952l-.427 2.83c.672.59 2.328 2.316 3.335 4.711.09.21.175.43.253.653z"],
    folder: ["M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"],
    globe: ["M2.0 12a10 10 0 1 0 20.0 0a10 10 0 1 0 -20.0 0", "M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20", "M2 12h20"],
    sun: ["M12 8a4 4 0 1 0 0 8a4 4 0 1 0 0-8", "M12 2v2", "M12 20v2", "m4.93 4.93 1.41 1.41", "m17.66 17.66 1.41 1.41",
      "M2 12h2", "M20 12h2", "m6.34 17.66-1.41 1.41", "m19.07 4.93-1.41 1.41"],
    moon: ["M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"],
    auto: ["M12 2v2", "M14.837 16.385a6 6 0 1 1-7.223-7.222c.624-.147.97.66.715 1.248a4 4 0 0 0 5.26 5.259c.589-.255 1.396.09 1.248.715",
      "M16 12a4 4 0 0 0-4-4", "m19 5-1.256 1.256", "M20 12h2"],
  };
  const FILLED = new Set(["obsidian"]);
  function icon(name) {
    const NS = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(NS, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("aria-hidden", "true");
    if (FILLED.has(name)) svg.classList.add("fill");
    ICONS[name].forEach((d) => { const p = document.createElementNS(NS, "path"); p.setAttribute("d", d); svg.appendChild(p); });
    return svg;
  }
  // A button with an icon and an optional short label (label text lives in .lbl).
  function ibtn(name, label, cls) {
    const b = el("button", "ib" + (cls ? " " + cls : ""));
    b.appendChild(icon(name));
    if (label) b.appendChild(el("span", "lbl", label));
    return b;
  }
  // Opens the file in the Obsidian app, next to the agent buttons; the API
  // sends the link only with obsidian.vault in the config.
  function obsidianLink(url) {
    const a = el("a", "ib ghost copy lbtn");
    a.href = url;
    a.title = "open in Obsidian";
    a.appendChild(icon("obsidian"));
    a.appendChild(el("span", "lbl", "obsidian"));
    return a;
  }
  function chip(text, cls, title) {
    const c = el("span", "chip" + (cls ? " " + cls : ""), text);
    if (title) c.title = title;
    return c;
  }

  // ---- API ----------------------------------------------------------------

  async function api(path, opts) {
    opts = opts || {};
    const headers = { Accept: "application/json" };
    if (opts.method && opts.method !== "GET") {
      headers["X-Baton-Token"] = session && session.csrf;
      if (opts.body !== undefined) headers["Content-Type"] = "application/json";
    }
    const res = await fetch(path, {
      method: opts.method || "GET",
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      credentials: "same-origin",
      cache: "no-store",
    });
    if (res.status === 401) {
      location.replace("/login");
      throw new Error("signed out");
    }
    const out = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = new Error(out.error || "HTTP " + res.status);
      err.status = res.status;
      throw err;
    }
    return out;
  }
  const taskPath = (t) => `/api/projects/${enc(t.project)}/tasks/${enc(t.task)}`;
  const keyOf = (t) => t.project + "|" + t.task;
  const tipKey = (tip) => tip.scope + "|" + tip.id;
  const command = (t) => "baton pickup " + t.project + " @" + t.task;

  async function copy(text, btn, codeEl, label) {
    const lbl = btn.querySelector(".lbl") || btn;
    const done = (msg) => {
      lbl.textContent = msg;
      btn.classList.add("ok");
      btn.replaceChild(icon("check"), btn.querySelector("svg"));
      setTimeout(() => {
        lbl.textContent = label;
        btn.classList.remove("ok");
        btn.replaceChild(icon("copy"), btn.querySelector("svg"));
      }, 1500);
    };
    try {
      await navigator.clipboard.writeText(text);
      return done("Copied");
    } catch (e) { /* fall through */ }
    try {
      const a = el("textarea", "offscreen");
      a.value = text;
      a.setAttribute("readonly", "");
      document.body.appendChild(a);
      a.select();
      const ok = document.execCommand("copy");
      a.remove();
      if (ok) return done("Copied");
    } catch (e) { /* fall through */ }
    codeEl.hidden = false;
    const r = document.createRange();
    r.selectNodeContents(codeEl);
    const sel = getSelection();
    sel.removeAllRanges();
    sel.addRange(r);
    done("Selected");
  }

  function copyButton(label, text, cls) {
    const btn = ibtn("copy", label, (cls || "ghost") + " copy");
    btn.title = "Copy: " + text;
    const code = el("code", "manual", text);
    code.hidden = true; // shown only when the command has to be copied by hand
    btn.addEventListener("click", (e) => { e.stopPropagation(); copy(text, btn, code, label); });
    return [btn, code];
  }

  // ---- task and tip rows --------------------------------------------------

  // Git context as breadcrumb segments: branch > commit (null without git info).
  function gitCrumbs(t) {
    const crumbs = el("div", "crumbs mono");
    const seg = (name, text, title) => {
      const c = el("span", "crumb " + name);
      c.appendChild(icon(name));
      c.appendChild(el("span", "", text));
      c.title = title;
      crumbs.appendChild(c);
    };
    if (t.branch) seg("branch", t.branch, "branch at handoff time");
    if (t.commit) seg("commit", t.commit, "commit at handoff time");
    return crumbs.childNodes.length ? crumbs : null;
  }

  const projectOf = (key) => data.projects.find((p) => p.key === key);
  const statusText = (x) => x ? (x.archived ? "done" : "active") : "missing";

  // full: the panel lists each fork; a card only counts them. shown: tasks in
  // the card's tree, whose links the indentation already shows.
  function chips(t, full, shown) {
    const box = el("div", "chips");
    shown = shown || new Set();
    if (t.archived && !(t.from && shown.has(t.from))) box.appendChild(chip("archived", "archived"));
    const same = projectOf(t.project).tasks;
    if (t.from && !shown.has(t.from)) {
      const p = same.find((x) => x.task === t.from);
      box.appendChild(taskLink(chip("↑ @" + t.from, p ? "link" : "missing", "fork of @" + t.from + " (" + statusText(p) + ")"), p));
    }
    const kids = same.filter((x) => x.from === t.task && !shown.has(x.task));
    if (full) kids.forEach((k) => box.appendChild(taskLink(chip("↓ @" + k.task, "link", "fork: " + k.title + " (" + statusText(k) + ")"), k)));
    else if (kids.length) {
      box.appendChild(taskLink(chip("↓ " + (kids.length === 1 ? "@" + kids[0].task : kids.length + " forks"), kids.length === 1 ? "link" : "",
        "forks: " + kids.map((k) => "@" + k.task + " (" + statusText(k) + ")").join(", ")), kids.length === 1 ? kids[0] : null));
    }
    return box;
  }
  // Makes a chip open task x (a click or Enter).
  function taskLink(c, x) {
    if (!x) return c;
    c.tabIndex = 0;
    c.setAttribute("role", "button");
    const go = (e) => { e.stopPropagation(); e.preventDefault(); openTask(x); };
    c.addEventListener("click", go);
    c.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") go(e); });
    return c;
  }
  function openTask(t) {
    if (t.archived && !state.archOpen.includes(t.project)) { state.archOpen.push(t.project); render(); }
    openPanel("t:" + keyOf(t), hue(t.project), projectWho(t.project), (body) => taskPanel(t, body));
  }

  // Card head: title and a "@task · chips" line; age and buttons on the right.
  function cardHead(title, name, cs, at, atTitle, buttons, dot) {
    const head = el("div", "head");
    if (dot) {
      const d = el("span", "nicon " + dot);
      d.title = dot === "done" ? "done (archived)" : "open";
      d.appendChild(icon(dot === "done" ? "cdone" : "copen"));
      head.appendChild(d);
    }
    const main = el("div", "hmain");
    main.appendChild(rich("div", "title", title || name));
    const sl = el("div", "sline");
    if (title && title !== name) sl.appendChild(hl(el("span", "task mono"), name));
    if (cs.childNodes.length) sl.appendChild(cs);
    if (sl.childNodes.length) main.appendChild(sl);
    head.appendChild(main);
    const side = el("div", "side");
    const a = el("span", "age", age(at));
    if (atTitle) a.title = atTitle;
    side.appendChild(a);
    buttons.forEach((b) => side.appendChild(b));
    head.appendChild(side);
    return [head, main];
  }
  function arrow() {
    const a = el("span", "arrow");
    a.appendChild(icon("right"));
    return a;
  }

  // ---- side panel ---------------------------------------------------------

  let lastFocus = null;
  function markOpen() {
    document.querySelectorAll("#list [data-key]").forEach((e) => e.classList.toggle("open", e.dataset.key === state.open));
  }
  function whoNode(avatar, name, sub, title) {
    const w = el("span", "who");
    if (title) w.title = title;
    const av = el("span", "avatar sm");
    typeof avatar === "string" ? (av.textContent = avatar) : av.appendChild(avatar);
    w.appendChild(av);
    w.appendChild(el("span", "pname", name));
    if (sub) w.appendChild(el("span", "ppath", sub));
    return w;
  }
  const dirDot = (p, key, cls) => {
    const d = el("span", "dirdot " + (p && p.dir ? "on" : "off") + (cls ? " " + cls : ""));
    d.title = p && p.dir ? "directory on this machine: " + p.dir : "no directory on this machine: baton path set " + key + " DIR";
    return d;
  };
  const projectWho = (key) => () => {
    const p = projectOf(key);
    return whoNode(initials(key), key, p && p.dir ? tilde(p.dir) : "", p && p.dir);
  };
  const globalWho = () => whoNode(icon("globe"), "Global tips", "every project", "");

  function openPanel(key, h, where, build) {
    if (!leaveEditor()) return;
    const p = $("#panel");
    p.style.setProperty("--h", h);
    $("#pwhere").textContent = "";
    $("#pwhere").appendChild(where());
    const body = $("#pbody");
    body.textContent = "";
    body.scrollTop = 0;
    build(body);
    state.open = key;
    markOpen();
    if (p.hidden) lastFocus = document.activeElement;
    p.hidden = false;
    $("#scrim").hidden = false;
    document.body.classList.add("noscroll");
    $("#pclose").focus();
  }
  function closePanel() {
    if ($("#panel").hidden || !leaveEditor()) return;
    $("#panel").hidden = true;
    $("#scrim").hidden = true;
    document.body.classList.remove("noscroll");
    state.open = null;
    markOpen();
    if (lastFocus && document.contains(lastFocus)) lastFocus.focus();
  }
  // A list row that opens the panel with Enter/Space or a click.
  function opener(head, open) {
    head.tabIndex = 0;
    head.setAttribute("role", "button");
    head.addEventListener("click", () => { if (!getSelection().toString()) open(); });
    head.addEventListener("keydown", (e) => {
      if (e.target === head && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); open(); }
    });
  }
  function panelHead(body, title, name, cs, at, atTitle) {
    body.appendChild(rich("h3", "ptitle2", title || name));
    const sl = el("div", "sline");
    if (title && title !== name) sl.appendChild(el("span", "task mono", name));
    if (cs.childNodes.length) sl.appendChild(cs);
    const a = el("span", "age", age(at));
    if (atTitle) a.title = atTitle;
    sl.appendChild(a);
    body.appendChild(sl);
  }
  const failed = (box) => (e) => { box.textContent = "Cannot load (" + e.message + ")."; };

  function diffView(lines) {
    const pre = el("pre", "diff");
    if (!lines.some((l) => l.op !== " ")) { pre.textContent = "No differences."; return pre; }
    lines.forEach((l) => pre.appendChild(el("span", l.op === "+" ? "add" : l.op === "-" ? "del" : "", l.op + " " + l.text + "\n")));
    return pre;
  }

  // The panel head shows the title; a first "# Title" heading in the text
  // would repeat it.
  // onCheck(line, checked, box) gets lines of the full body.
  function bodyView(body, title, project, onCheck) {
    const m = /^\s*# (.*)\n?/.exec(body);
    const cut = m && m[1].trim() === String(title).trim() ? m[0] : "";
    const skip = cut.split("\n").length - 1;
    return window.renderMarkdown(body.slice(cut.length), { ref: refs(project), onCheck: onCheck && ((n, on, box) => onCheck(n + skip, on, box)) });
  }

  // Links in a text of project: @name to a task there, [[name]] to a task or
  // else a tip (the project's, then global). Unknown names stay text.
  function refs(project) {
    return (name, label) => {
      const p = projectOf(project);
      const x = p && p.tasks.find((t) => t.task === name);
      const tip = !x && label !== "@" + name && (p ? p.tips : []).concat(data.global).find((t) => t.id === name);
      if (!x && !tip) return null;
      const a = el("a", "ref" + (x && x.archived ? " done" : ""), label);
      a.href = "#";
      a.title = x ? "@" + x.task + ": " + x.title + " (" + statusText(x) + ")" : "tip: " + tip.title;
      a.addEventListener("click", (e) => { e.preventDefault(); x ? openTask(x) : openTip(tip); });
      return a;
    };
  }

  // Latest / History tabs. Version n: 0 is the current handoff, 1.. history (newest first).
  function details(t, d, box, actions) {
    const tabs = el("div", "seg tabs");
    tabs.setAttribute("role", "tablist");
    const view = el("div");
    view.setAttribute("role", "tabpanel");
    const tab = (text) => { const b = el("button", "", text); b.setAttribute("role", "tab"); tabs.appendChild(b); return b; };
    const latest = tab("Latest");
    const hist = tab("History");
    if (d.history.length) hist.appendChild(el("span", "count", String(d.history.length + 1)));
    const bar = el("div", "dbar");
    bar.appendChild(tabs);
    actions.forEach((a) => bar.appendChild(a));
    const crumbs = gitCrumbs(d.handoff);
    if (crumbs) box.appendChild(crumbs);
    box.appendChild(bar);
    box.appendChild(view);
    const select = (b) => [latest, hist].forEach((x) => x.setAttribute("aria-selected", String(x === b)));

    function showLatest() {
      view.textContent = "";
      const stale = d.staleness_lines || [];
      if (stale.length) {
        const n = el("div", "note");
        n.appendChild(el("strong", "", "Code changed since this handoff"));
        stale.forEach((l) => n.appendChild(el("div", "mono", l)));
        view.appendChild(n);
      }
      view.appendChild(bodyView(d.body, d.handoff.title, t.project, t.archived ? null : tick));
    }
    function showHistory() {
      view.textContent = "";
      const list = el("div", "history");
      const rows = [d.handoff].concat(d.history);
      let current = null; // the button whose result is open
      rows.forEach((v, n) => {
        const row = el("div", "hrow");
        const a = el("span", "age", age(v.created));
        a.title = when(v.created);
        row.appendChild(a);
        row.appendChild(rich("span", "htitle", (n ? "" : "current · ") + v.title));
        const btns = el("span", "hbtns");
        const panel = el("div", "hpanel");
        panel.hidden = true;
        const toggle = (btn, loader) => {
          if (current) current.setAttribute("aria-pressed", "false");
          if (current === btn) { current = null; panel.hidden = true; return; }
          list.querySelectorAll(".hpanel").forEach((p) => { p.hidden = true; });
          current = btn;
          btn.setAttribute("aria-pressed", "true");
          panel.hidden = false;
          panel.textContent = "Loading…";
          loader().then((node) => {
            panel.textContent = "";
            panel.appendChild(node);
            panel.scrollIntoView({ block: "nearest" });
          }).catch(failed(panel));
        };
        // Every row has View and Diff so the buttons line up; the first
        // version has nothing to diff against.
        const open = el("button", "", "View");
        open.addEventListener("click", () => toggle(open, async () =>
          bodyView(n ? (await api(taskPath(t) + "/history/" + n)).body : d.body, v.title, t.project)));
        btns.appendChild(open);
        const b = el("button", "", "Diff");
        if (n + 1 < rows.length) {
          b.title = "changes since the previous version";
          b.addEventListener("click", () => toggle(b, async () =>
            diffView((await api(taskPath(t) + `/diff?from=${n + 1}&to=${n}`)).lines)));
        } else {
          b.disabled = true;
          b.title = "first version: nothing to compare with";
        }
        btns.appendChild(b);
        if (n) {
          const r = el("button", "", "Restore");
          r.title = "make this version current; the current one stays in History";
          r.addEventListener("click", async () => {
            if (!confirm("Make this version of @" + t.task + " current?")) return;
            r.disabled = true;
            try {
              await saved(t.project, await api(taskPath(t) + "/history/" + n + "/restore", { method: "POST", body: { version: d.version } }));
            } catch (e) {
              r.disabled = false;
              alert(e.status === 409 ? "The handoff changed since you opened it; reopen it and try again." : "Failed: " + e.message);
            }
          });
          btns.appendChild(r);
        }
        row.appendChild(btns);
        list.appendChild(row);
        list.appendChild(panel);
      });
      view.appendChild(list);
    }
    // A ticked step is saved in place (no History entry), like an edit by hand.
    async function tick(line, on, box) {
      box.disabled = true;
      try {
        const r = await api(taskPath(t) + "/check", { method: "POST", body: { line, checked: on, version: d.version } });
        d.version = r.version;
        d.body = r.body;
      } catch (e) {
        box.checked = !on;
        alert(e.status === 409 ? "The handoff changed since you opened it; reopen it and try again." : "Failed: " + e.message);
      } finally { box.disabled = false; }
    }
    latest.addEventListener("click", () => { select(latest); showLatest(); });
    hist.addEventListener("click", () => { select(hist); showHistory(); });
    select(latest);
    showLatest();
  }

  function taskPanel(t, body) {
    panelHead(body, t.title === t.task ? "" : t.title, "@" + t.task, chips(t, true), t.created, when(t.created));
    const box = el("div", "details");
    box.textContent = "Loading…";
    body.appendChild(box);
    api(taskPath(t)).then((d) => {
      box.textContent = "";
      // "[claude] [opencode]": a copy-pickup button per agent.<name> line in
      // the config (none without them), the default one first and highlighted.
      const pk = (d.pickup || []).slice().sort((a, b) => a.command.includes(" --agent ") - b.command.includes(" --agent "));
      const row = el("div", "agents");
      const codes = [];
      pk.forEach((x, i) => {
        const [b, c] = copyButton(x.agent, x.command, i ? "" : "primary");
        if (!i) b.title += " (default agent)";
        row.appendChild(b);
        codes.push(c);
      });
      if (d.obsidian) row.appendChild(obsidianLink(d.obsidian));
      const acts = [];
      const edit = ibtn("edit", "Edit");
      edit.title = "edit the handoff (e); saved like baton save, the current text goes to History";
      edit.dataset.hotkey = "e";
      edit.addEventListener("click", () => editTask(t, d, box));
      acts.push(edit);
      const fork = ibtn("fork", "Fork");
      fork.title = "start a task from @" + t.task + " (f)";
      fork.dataset.hotkey = "f";
      fork.addEventListener("click", () => newTask(t.project, t, d.body));
      acts.push(fork);
      const ren = ibtn("type", "Rename", "ren");
      ren.title = "rename @" + t.task;
      ren.addEventListener("click", async () => {
        const name = prompt("New name for @" + t.task + ":", t.task);
        const next = name && name.trim().replace(/^@/, "");
        if (!next || next === t.task) return;
        ren.disabled = true;
        try {
          await api(taskPath(t) + "/rename", { method: "POST", body: { name: next } });
          closePanel();
          await load();
        } catch (e) { ren.disabled = false; alert("Failed: " + e.message); }
      });
      acts.push(ren);
      const done = !t.archived;
      const act = ibtn(done ? "check" : "undo", done ? "Done" : "Restore", "act");
      act.title = done ? "archive the task (like baton done " + t.task + ")" : "move the task back to active";
      act.addEventListener("click", async () => {
        if (!confirm((done ? "Archive" : "Restore") + " @" + t.task + "?")) return;
        act.disabled = true;
        try {
          await api(taskPath(t) + (done ? "/done" : "/restore"), { method: "POST" });
          closePanel();
          await load();
        } catch (e) { act.disabled = false; alert("Failed: " + e.message); }
      });
      acts.push(act);
      details(t, d, box, acts);
      if (notice && notice.key === keyOf(t)) {
        const n = el("div", "note");
        n.appendChild(el("strong", "", "Saved with warnings"));
        notice.warnings.forEach((w) => n.appendChild(el("div", "", w)));
        box.insertBefore(n, box.firstChild);
      }
      notice = null;
      // Under the git crumbs, above the tabs.
      const bar = box.querySelector(".dbar");
      if (row.childNodes.length) box.insertBefore(row, bar);
      codes.forEach((c) => box.insertBefore(c, bar));
    }).catch(failed(box));
  }

  // ---- editor -------------------------------------------------------------

  let tpl = null; // { body, max_chars, max_line } from /api/template
  const template = async () => tpl || (tpl = await api("/api/template"));
  let dirty = null; // () => true while the open editor has unsaved text
  let notice = null; // { key, warnings } shown once by the next task panel
  function leaveEditor() {
    if (dirty && dirty() && !confirm("Discard unsaved changes?")) return false;
    dirty = null;
    return true;
  }
  window.addEventListener("beforeunload", (e) => { if (dirty && dirty()) e.preventDefault(); });

  // A labelled one-line input appended to box.
  function field(box, label, value, cls) {
    const l = el("label", "field");
    l.appendChild(el("span", "", label));
    const i = el("input", "ename" + (cls ? " " + cls : ""));
    i.value = value || "";
    i.autocomplete = "off";
    l.appendChild(i);
    box.appendChild(l);
    return i;
  }

  // Ctrl+S (Cmd+S) in the editor fields saves; the panel box is reused, so
  // the keys are bound to the fields, which every editor makes anew.
  function onSaveKey(fields, save) {
    fields.forEach((f) => f && f.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); save(); }
    }));
  }

  // Mirrors store.SizeWarnings: fenced code is not counted.
  function sizeOf(text) {
    let chars = 0, long = 0, fence = false;
    text.split("\n").forEach((l) => {
      if (l.trim().startsWith("```")) { fence = !fence; return; }
      if (fence) return;
      const n = Array.from(l).length;
      chars += n + 1;
      if (n > tpl.max_line) long++;
    });
    return { chars, long };
  }

  // Edit / Preview tabs over a textarea, a size meter, Save and Cancel
  // (Ctrl+S saves). o: { text, name (a task-name field when set), save(text, name, msg), cancel }.
  function editor(box, o) {
    box.textContent = "";
    let name = null;
    if (o.name !== undefined) {
      name = el("input", "ename mono");
      name.value = o.name;
      name.placeholder = "task-name";
      name.autocomplete = "off";
      name.spellcheck = false;
      name.setAttribute("aria-label", "Task name");
      box.appendChild(name);
    }
    const tabs = el("div", "seg tabs");
    tabs.setAttribute("role", "tablist");
    const tab = (text) => { const b = el("button", "", text); b.setAttribute("role", "tab"); tabs.appendChild(b); return b; };
    const write = tab("Edit"), look = tab("Preview");
    const bar = el("div", "dbar");
    bar.appendChild(tabs);
    const meter = el("span", "meter");
    bar.appendChild(meter);
    box.appendChild(bar);
    const ta = el("textarea", "editor mono");
    ta.value = o.text;
    ta.spellcheck = false;
    ta.setAttribute("aria-label", "Handoff Markdown");
    const view = el("div", "preview");
    const msg = el("div");
    box.appendChild(msg);
    box.appendChild(ta);
    box.appendChild(view);
    const select = (b) => {
      [write, look].forEach((x) => x.setAttribute("aria-selected", String(x === b)));
      ta.hidden = b !== write;
      view.hidden = b === write;
      if (b === look) { view.textContent = ""; view.appendChild(window.renderMarkdown(ta.value)); }
    };
    write.addEventListener("click", () => { select(write); ta.focus(); });
    look.addEventListener("click", () => select(look));
    select(write);
    const count = () => {
      const z = sizeOf(ta.value);
      meter.textContent = z.chars + " / " + tpl.max_chars + (z.long ? " · " + z.long + " lines over " + tpl.max_line : "");
      meter.title = "characters outside code blocks; the budget is a warning, not a limit";
      meter.classList.toggle("over", z.chars > tpl.max_chars || z.long > 0);
    };
    ta.addEventListener("input", count);
    count();
    const row = el("div", "actions");
    const save = el("button", "primary", "Save");
    save.title = "save (Ctrl+S)";
    const cancel = el("button", "", "Cancel");
    row.appendChild(save);
    row.appendChild(cancel);
    box.appendChild(row);
    const start = o.text, startName = o.name;
    dirty = () => ta.value !== start || (name !== null && name.value !== startName);
    async function doSave() {
      if (save.disabled) return;
      save.disabled = true;
      try {
        await o.save(ta.value, name ? name.value.trim().replace(/^@/, "") : "", msg);
      } catch (e) {
        msg.textContent = "";
        msg.appendChild(el("div", "note", "Not saved: " + e.message));
      } finally { save.disabled = false; }
    }
    save.addEventListener("click", doSave);
    onSaveKey([name, ta], doSave);
    cancel.addEventListener("click", () => { if (leaveEditor()) o.cancel(); });
    (name || ta).focus();
  }

  // After a save: refresh the list and show the task, with size warnings once.
  async function saved(project, r) {
    dirty = null;
    const t = Object.assign({ project }, r.handoff);
    if (r.warnings.length) notice = { key: keyOf(t), warnings: r.warnings };
    await load();
    openTask(t);
  }

  async function editTask(t, d, box) {
    try { await template(); } catch (e) { alert("Failed: " + e.message); return; }
    let version = d.version;
    editor(box, {
      text: d.body,
      cancel: () => openTask(t),
      save: async (text, _, msg) => {
        try {
          await saved(t.project, await api(taskPath(t), { method: "PUT", body: { body: text, version } }));
        } catch (e) {
          if (e.status !== 409) throw e;
          // An agent or a sync changed the file: show it, keep the text; the
          // next Save replaces it and the replaced version stays in History.
          const cur = await api(taskPath(t));
          version = cur.version;
          msg.textContent = "";
          const n = el("div", "note");
          n.appendChild(el("strong", "", "Changed since you opened it"));
          n.appendChild(el("div", "", "Save again to replace the version below; it stays in History."));
          msg.appendChild(n);
          const latest = el("details", "latest");
          latest.appendChild(el("summary", "", "Latest version (" + age(cur.handoff.created) + ")"));
          latest.appendChild(window.renderMarkdown(cur.body));
          msg.appendChild(latest);
        }
      },
    });
  }

  // A new task (or tip) in project, or a fork of parent starting from its text.
  function newTask(project, parent, text) {
    openPanel("new:" + project, hue(project), projectWho(project), (body) => {
      body.appendChild(el("h3", "ptitle2", parent ? "Fork of @" + parent.task : "New"));
      const box = el("div", "details");
      const startEditor = (body, name) => template().then((tp) => editor(box, {
        name: name !== undefined ? name : parent ? parent.task + "-" : "",
        text: body !== undefined ? body : parent ? text : tp.body,
        cancel: () => (parent ? openTask(parent) : closePanel()),
        save: async (body, name) => {
          if (!name) throw new Error("the task needs a name");
          try {
            await saved(project, await api(`/api/projects/${enc(project)}/tasks`, { method: "POST",
              body: { task: name, from: parent ? parent.task : "", body } }));
          } catch (e) {
            if (e.status === 409) throw new Error("@" + name + " already exists");
            throw e;
          }
        },
      })).catch(failed(box));
      const startTask = () => (parent ? startEditor() : template().then(() => quickForm(box, project, startEditor)).catch(failed(box)));
      const startTip = () => template().then(() => tipForm(box, { scope: project })).catch(failed(box));
      if (!parent) {
        const tabs = el("div", "seg tabs newkind");
        tabs.setAttribute("role", "tablist");
        [["Task", startTask], ["Tip", startTip]].forEach(([label, start], i) => {
          const b = el("button", "", label);
          b.setAttribute("role", "tab");
          b.setAttribute("aria-selected", String(!i));
          b.addEventListener("click", () => {
            if (b.getAttribute("aria-selected") === "true" || !leaveEditor()) return;
            tabs.querySelectorAll("button").forEach((x) => x.setAttribute("aria-selected", String(x === b)));
            box.textContent = "Loading…";
            start();
          });
          tabs.appendChild(b);
        });
        body.appendChild(tabs);
      }
      box.textContent = "Loading…";
      body.appendChild(box);
      startTask();
    });
  }

  // Title, name, goal and a first step: enough for a task an agent can pick
  // up. The name follows the title until it is edited; full(text, name)
  // continues in the editor.
  function quickForm(box, project, full) {
    box.textContent = "";
    const title = field(box, "Title", "");
    const name = field(box, "Name", "", "mono");
    name.placeholder = "from the title";
    name.spellcheck = false;
    const goal = field(box, "Goal", "");
    goal.placeholder = "what the task achieves, one sentence";
    const step = field(box, "First step", "");
    step.placeholder = "optional; default: Plan the work.";
    const inputs = [title, name, goal, step];
    const msg = el("div");
    box.appendChild(msg);
    const row = el("div", "actions");
    const create = el("button", "primary", "Create");
    create.title = "create the task (Enter)";
    const more = el("button", "", "Full editor");
    more.title = "continue in the editor with the whole template";
    const cancel = el("button", "", "Cancel");
    [create, more, cancel].forEach((b) => row.appendChild(b));
    box.appendChild(row);
    dirty = () => inputs.some((i) => i.value.trim());

    let auto = true, seq = 0, timer = 0;
    const suggest = async () => {
      const my = ++seq;
      const r = await api(`/api/projects/${enc(project)}/slug?title=${enc(title.value)}`);
      if (auto && my === seq) name.value = r.task;
    };
    name.addEventListener("input", () => { auto = !name.value; });
    title.addEventListener("input", () => {
      if (!auto) return;
      clearTimeout(timer);
      timer = setTimeout(() => suggest().catch(() => {}), 250);
    });
    const text = () => "# " + title.value.trim() + "\n\n## Goal\n" + goal.value.trim() +
      "\n\n## State\nNot started.\n\n## Next steps\n1. [ ] " + (step.value.trim() || "Plan the work.") + "\n";
    const fail = (t) => { msg.textContent = ""; msg.appendChild(el("div", "note", t)); };

    async function doCreate() {
      if (create.disabled) return;
      if (!title.value.trim() || !goal.value.trim()) { fail("Title and Goal are needed; the rest can wait."); return; }
      create.disabled = true;
      try {
        clearTimeout(timer);
        if (auto) await suggest();
        const task = name.value.trim().replace(/^@/, "");
        if (!task) throw new Error("the task needs a name");
        try {
          await saved(project, await api(`/api/projects/${enc(project)}/tasks`, { method: "POST", body: { task, body: text() } }));
        } catch (e) {
          throw e.status === 409 ? new Error("@" + task + " already exists") : e;
        }
      } catch (e) { fail("Not created: " + e.message); } finally { create.disabled = false; }
    }
    create.addEventListener("click", doCreate);
    inputs.forEach((i) => i.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.isComposing) { e.preventDefault(); doCreate(); }
    }));
    onSaveKey(inputs, doCreate);
    more.addEventListener("click", () => {
      const body = title.value.trim() || goal.value.trim() ? text() : undefined;
      dirty = null;
      full(body, name.value.trim());
    });
    cancel.addEventListener("click", () => { if (leaveEditor()) closePanel(); });
    title.focus();
  }

  function openTip(tip) {
    const g = tip.scope === GLOBAL;
    openPanel("p:" + tipKey(tip), g ? 200 : hue(tip.scope), g ? globalWho : projectWho(tip.scope), (body) => tipPanel(tip, body));
  }
  async function tipSaved(r) {
    dirty = null;
    if (r.warnings && r.warnings.length) notice = { key: "p:" + tipKey(r.tip || r), warnings: r.warnings };
    await load();
    openTip(r.tip || r);
  }

  // Title, when, keywords and the Tip:/Why:/Verify: text. Editing shows the
  // diff first (tips have no history); a new tip picks its scope.
  // o: { tip, version } to edit, or { scope } (a project) for a new one.
  function tipForm(box, o) {
    box.textContent = "";
    const t = o.tip;
    let scope = null;
    if (!t) {
      const l = el("label", "field");
      l.appendChild(el("span", "", "Scope"));
      scope = el("select", "ename");
      [[o.scope, "project " + o.scope], [GLOBAL, "global: every project"]].forEach(([v, text]) => {
        const op = el("option", "", text);
        op.value = v;
        scope.appendChild(op);
      });
      l.appendChild(scope);
      box.appendChild(l);
    }
    const title = field(box, "Title", t ? t.title : "");
    const whenIn = field(box, "When", t ? t.when : "");
    whenIn.placeholder = "the situation where it applies";
    const kw = field(box, "Keywords", t ? (t.keywords || []).join(", ") : "", "mono");
    kw.placeholder = "comma-separated";
    const ta = el("textarea", "editor tip mono");
    ta.value = t ? t.body : "Tip: \nWhy: \nVerify: \n";
    ta.spellcheck = false;
    ta.setAttribute("aria-label", "Tip text");
    const msg = el("div");
    box.appendChild(msg);
    box.appendChild(ta);
    const row = el("div", "actions");
    const save = el("button", "primary", t ? "Review" : "Save");
    save.title = t ? "show the changes before saving (Ctrl+S)" : "save (Ctrl+S)";
    const cancel = el("button", "", "Cancel");
    row.appendChild(save);
    row.appendChild(cancel);
    box.appendChild(row);
    const inputs = [title, whenIn, kw, ta];
    const start = inputs.map((i) => i.value).join("\0");
    dirty = () => inputs.map((i) => i.value).join("\0") !== start;
    let confirmed = false; // Review showed this text; the next click saves
    inputs.forEach((i) => i.addEventListener("input", () => {
      if (confirmed) { confirmed = false; save.textContent = "Review"; msg.textContent = ""; }
    }));
    const payload = () => ({ title: title.value, when: whenIn.value, body: ta.value,
      keywords: kw.value.split(",").map((k) => k.trim()).filter(Boolean) });
    const fail = (text) => { msg.textContent = ""; msg.appendChild(el("div", "note", text)); };
    async function doSave() {
      if (save.disabled) return;
      save.disabled = true;
      try {
        if (!t) {
          await tipSaved(await api("/api/tips", { method: "POST", body: Object.assign({ scope: scope.value }, payload()) }));
        } else if (!confirmed) {
          const r = await api(tipPath(t), { method: "PUT", body: Object.assign({ version: o.version, preview: true }, payload()) });
          msg.textContent = "";
          r.warnings.forEach((w) => msg.appendChild(el("div", "note", w)));
          msg.appendChild(diffView(r.lines));
          confirmed = true;
          save.textContent = "Save";
          save.title = "save these changes (Ctrl+S)";
        } else {
          await tipSaved(await api(tipPath(t), { method: "PUT", body: Object.assign({ version: o.version }, payload()) }));
        }
      } catch (e) {
        fail(e.status === 409 ? "The tip changed since you opened it; copy your text, cancel and reopen it." : "Not saved: " + e.message);
      } finally { save.disabled = false; }
    }
    save.addEventListener("click", doSave);
    onSaveKey(inputs, doSave);
    cancel.addEventListener("click", () => { if (leaveEditor()) (t ? openTip(t) : closePanel()); });
    title.focus();
  }

  // Tips: read and delete here; `baton tips` does the rest.
  function tipChips(tip) {
    const cs = el("span", "chips");
    if (tip.status !== "active") cs.appendChild(chip(tip.status, tip.status === "verified" ? "used" : "warn"));
    if (tip.scope !== GLOBAL && state.proj === GLOBAL) cs.appendChild(chip(tip.scope));
    (tip.env || []).forEach((e) => cs.appendChild(chip("env: " + e)));
    if (tip.origin === "web") cs.appendChild(chip("from web", "", "learned from web pages, issues or foreign code"));
    return cs;
  }
  const tipDate = (tip) => tip.updated || (tip.source && tip.source[2]) || "";
  const tipPath = (tip) => `/api/tips/${enc(tip.scope)}/${enc(tip.id)}`;
  function tipCard(tip, where, h) {
    const c = el("div", "item" + (tip.status === "active" || tip.status === "verified" ? "" : " dim"));
    c.dataset.key = "p:" + tipKey(tip);
    const [head] = cardHead(tip.title, tip.id, tipChips(tip), tipDate(tip), tipDate(tip) && "saved " + when(tipDate(tip)), [arrow()]);
    c.appendChild(head);
    opener(head, () => openPanel(c.dataset.key, h, where, (body) => tipPanel(tip, body)));
    return c;
  }
  function tipPanel(tip, body) {
    panelHead(body, tip.title, tip.id, tipChips(tip), tipDate(tip), "");
    const box = el("div", "details");
    box.textContent = "Loading…";
    body.appendChild(box);
    api(tipPath(tip)).then((d) => {
      const t = d.tip;
      box.textContent = "";
      if (d.obsidian) {
        const row = el("div", "agents");
        row.appendChild(obsidianLink(d.obsidian));
        box.appendChild(row);
      }
      // "Tip: / Why: / Verify:" lines are separate paragraphs with bold labels.
      box.appendChild(window.renderMarkdown(String(t.body || "").replace(/\n(?=[A-Z][a-z]+: )/g, "\n\n")
        .replace(/^([A-Z][a-z]+): /gm, "**$1:** "), { ref: refs(t.scope) }));
      const meta = (label, v) => { if (v && v.length) box.appendChild(rich("div", "tipmeta", label + ": " + (Array.isArray(v) ? v.join(", ") : v))); };
      meta("When", t.when);
      meta("Keywords", t.keywords);
      meta("Cites", t.cites);
      meta("Env", t.env);
      meta("Origin", t.origin);
      meta("Source", (t.source || []).join(" "));
      meta("Verified", d.verified);
      meta("Superseded by", d.superseded_by);
      const row = el("div", "actions");
      row.appendChild(el("div", "file", t.scope + "/" + t.id));
      const mark = async (btn, path, body) => {
        btn.disabled = true;
        try { tipSaved(await api(tipPath(t) + path, { method: "POST", body })); } catch (e) { btn.disabled = false; alert("Failed: " + e.message); }
      };
      if (t.status !== "verified") {
        const ok = ibtn("check", "Verified");
        ok.title = "its Verify step passed (like baton tips verified)";
        ok.addEventListener("click", () => mark(ok, "/verified"));
        row.appendChild(ok);
      }
      if (t.status !== "refuted") {
        const no = ibtn("ban", "Refuted");
        no.title = "it is wrong; the reason goes into the tip (like baton tips refuted)";
        no.addEventListener("click", () => {
          const why = prompt("Why is " + t.id + " wrong?");
          if (why && why.trim()) mark(no, "/refuted", { why: why.trim() });
        });
        row.appendChild(no);
      }
      const edit = ibtn("edit", "Edit");
      edit.title = "edit the title, when, keywords and text (e)";
      edit.dataset.hotkey = "e";
      edit.addEventListener("click", () => template().then(() => tipForm(box, { tip: t, version: d.version })).catch((e) => alert("Failed: " + e.message)));
      row.appendChild(edit);
      const del = ibtn("trash", "Delete");
      del.title = "delete the tip file";
      del.addEventListener("click", async () => {
        if (!confirm("Delete tip " + t.id + "?")) return;
        del.disabled = true;
        try {
          await api(tipPath(t), { method: "DELETE" });
          closePanel();
          await load();
        } catch (e) { del.disabled = false; alert("Failed: " + e.message); }
      });
      row.appendChild(del);
      box.appendChild(row);
      if (notice && notice.key === "p:" + tipKey(t)) {
        const n = el("div", "note");
        n.appendChild(el("strong", "", "Saved with warnings"));
        notice.warnings.forEach((w) => n.appendChild(el("div", "", w)));
        box.insertBefore(n, box.firstChild);
      }
      notice = null;
    }).catch(failed(box));
  }
  // Tips, active and verified first, newest first (the date the card shows).
  function tipsList(tips, where, h) {
    const rank = (t) => (t.status === "active" || t.status === "verified" ? 0 : 1);
    const at = (t) => Date.parse(tipDate(t)) || 0;
    return tips.slice().sort((a, b) => rank(a) - rank(b) || at(b) - at(a)).map((t) => tipCard(t, where, h));
  }

  // ---- task trees ---------------------------------------------------------

  // Tasks as a forest of nodes: a task, then its forks (newest subtree first).
  // A fork whose parent is not in the list (other section, filtered out) is a root.
  function forest(tasks) {
    const names = new Set(tasks.map((t) => t.task)), kids = {}, done = new Set();
    const isRoot = (t) => !t.from || !names.has(t.from) || t.from === t.task;
    tasks.forEach((t) => { if (!isRoot(t)) (kids[t.from] = kids[t.from] || []).push(t); });
    const build = (t) => {
      done.add(t.task);
      const sub = (kids[t.task] || []).filter((k) => !done.has(k.task)).map(build);
      return { t, kids: sub, last: sub.reduce((m, k) => (k.last > m ? k.last : m), t.created) };
    };
    const order = (list) => { list.sort((a, b) => (b.last > a.last ? 1 : -1)); list.forEach((n) => order(n.kids)); return list; };
    const roots = tasks.filter(isRoot).map(build);
    tasks.forEach((t) => { if (!done.has(t.task)) roots.push(build(t)); }); // cycles: no root leads there
    return order(roots);
  }
  // marks: dim archived forks with a check (under an active task, not in Archived).
  function cards(tasks, marks) {
    const shown = new Set(tasks.map((t) => t.task));
    const node = (n, depth) => {
      const w = el("div", "node" + (depth ? " fork" : "") + (n.kids.length ? " parent" : "") +
        (marks && depth && n.t.archived ? " done" : ""));
      w.style.setProperty("--depth", depth);
      w.appendChild(card(n.t, marks && depth, shown));
      n.kids.forEach((k) => w.appendChild(node(k, depth + 1)));
      return w;
    };
    return forest(tasks).map((n) => node(n, 0));
  }
  function card(t, depth, shown) {
    const c = el("div", "item");
    c.dataset.key = "t:" + keyOf(t);
    const [copyBtn, code] = copyButton("Copy", command(t));
    const [head, main] = cardHead(t.title === t.task ? "" : t.title, "@" + t.task, chips(t, false, shown),
      t.created, when(t.created), [copyBtn, arrow()], depth && t.archived ? "done" : "open");
    const snippet = hits.get(keyOf(t));
    if (snippet) main.appendChild(hl(el("div", "snippet"), snippet));
    c.appendChild(head);
    c.appendChild(code);
    opener(head, () => openTask(t));
    return c;
  }

  // A stable hue per project name for its accent colour and avatar.
  function hue(name) {
    let h = 0;
    for (const ch of name) h = (h * 31 + ch.codePointAt(0)) % 360;
    return h;
  }
  function initials(name) {
    const w = name.replace(/^[^\p{L}\p{N}]+/u, "").split(/[^\p{L}\p{N}]+/u).filter(Boolean);
    return ((w.length > 1 ? w[0][0] + w[1][0] : (w[0] || "?").slice(0, 2)) || "?").toUpperCase();
  }

  // ---- layout -------------------------------------------------------------

  // The sidebar pick: "all", a project or GLOBAL; kept in the URL hash
  // ("#global-tips", "#<project>", none for All) so a reload keeps it.
  function fromHash() {
    let h = location.hash.slice(1);
    try { h = decodeURIComponent(h); } catch (e) { /* a malformed hash */ }
    return !h ? "all" : h === "global-tips" ? GLOBAL : h;
  }
  // The pick is kept in localStorage: the dashboard opens where it was left.
  function remember(v) {
    try { if (v === "all") localStorage.removeItem("baton.proj"); else localStorage.setItem("baton.proj", v); } catch (e) { /* private mode */ }
  }
  function pick(v) {
    const h = v === "all" ? "" : "#" + (v === GLOBAL ? "global-tips" : enc(v));
    if (h !== location.hash) history.pushState(null, "", h || location.pathname + location.search);
    state.proj = v;
    remember(v);
    render();
    window.scrollTo(0, 0);
  }
  // Sidebar and its <select> twin (narrow screens): All, the projects, Global tips.
  // A count is the active tasks, or everything that matches while searching.
  function renderNav(rows, nglobal, view, searching) {
    const nav = $("#nav"), sel = $("#navsel");
    nav.textContent = "";
    sel.textContent = "";
    const n = (r) => (searching ? r.tasks.length + r.ptips.length : r.act.length);
    const add = (v, label, num, lead, title) => {
      const b = el("button", "nitem");
      b.appendChild(lead);
      b.appendChild(el("span", "nname", label));
      b.appendChild(el("span", "count", String(num)));
      if (title) b.title = title;
      if (v === view) b.setAttribute("aria-current", "page");
      b.addEventListener("click", () => pick(v));
      nav.appendChild(b);
      const o = el("option", "", label + " (" + num + ")");
      o.value = v;
      o.selected = v === view;
      sel.appendChild(o);
    };
    add("all", "All", rows.reduce((s, r) => s + n(r), 0), icon("folder"), "every project in one column");
    if (rows.length) nav.appendChild(el("div", "nsep"));
    rows.forEach((r) => {
      const dot = el("span", "dot");
      dot.style.setProperty("--h", hue(r.p.key));
      add(r.p.key, r.p.key, n(r), dot, r.p.dir || "");
    });
    if (nglobal) {
      nav.appendChild(el("div", "nsep"));
      add(GLOBAL, "Global tips", nglobal, icon("globe"), "tips that hold in every project");
    }
  }

  function render() {
    if (!data) return;
    const q = state.q.trim().toLowerCase();
    const matches = (t) => !q || hits.has(keyOf(t)) ||
      [t.task, t.title, t.project].some((f) => f && f.toLowerCase().includes(q));
    const tipMatches = (t) => !q || tipHits.has(tipKey(t)) ||
      [t.id, t.title, t.when, (t.keywords || []).join(" ")].some((f) => f && f.toLowerCase().includes(q));
    $("#ver").textContent = "baton " + ((session && session.version) || "");
    $("#root").textContent = tilde(data.root);

    const list = $("#list");
    list.textContent = "";
    const items = (nodes) => { const box = el("div", "items"); nodes.forEach((n) => box.appendChild(n)); return box; };

    // Projects with something to show (a search hides the rest, in the sidebar too).
    const rows = data.projects.map((p) => {
      const tasks = p.tasks.filter(matches);
      return { p, tasks, ptips: p.tips.filter(tipMatches), act: tasks.filter((t) => !t.archived) };
    }).filter((r) => r.tasks.length || r.ptips.length || (!q && r.p.conflicts.length));
    const gtips = data.global.filter(tipMatches);
    // The pick falls back to All while it has nothing to show (a search, a stale hash).
    let view = state.proj;
    if (view === GLOBAL ? !gtips.length : view !== "all" && !rows.some((r) => r.p.key === view)) view = "all";
    renderNav(rows, gtips.length, view, !!q);

    // Repository: a host icon next to the project name, the address in the tooltip.
    const repoLink = (url) => {
      const host = url.replace(/^https:\/\//, "").split("/")[0];
      const a = el("a", "prepo");
      a.href = url;
      a.target = "_blank";
      a.rel = "noopener noreferrer";
      a.title = url.replace(/^https:\/\//, "");
      a.setAttribute("aria-label", "repository " + a.title);
      a.appendChild(icon(host === "github.com" ? "github" : /(^|\.)gitlab\./.test(host) ? "gitlab" : "git"));
      return a;
    };
    const projectSection = ({ p, tasks, ptips, act }) => {
      // Done forks stay under their active parent (at any depth); the rest go to Archived.
      const tree = act.slice(), inTree = new Set(act.map((t) => t.task));
      for (let grew = true; grew;) {
        grew = false;
        tasks.forEach((t) => {
          if (t.archived && t.from && inTree.has(t.from) && !inTree.has(t.task)) { tree.push(t); inTree.add(t.task); grew = true; }
        });
      }
      const arch = tasks.filter((t) => t.archived && !tree.includes(t));
      const h = hue(p.key);

      const sec = el("section", "project");
      sec.style.setProperty("--h", h);
      const head = el("h2");
      if (p.dir) head.title = p.dir;
      head.appendChild(el("span", "avatar", initials(p.key)));
      const pt = el("span", "ptitle");
      const prow = el("span", "prow");
      prow.appendChild(hl(el("span", "pname"), p.key));
      if (p.repo) prow.appendChild(repoLink(p.repo));
      pt.appendChild(prow);
      if (p.dir) pt.appendChild(el("span", "ppath", tilde(p.dir)));
      head.appendChild(pt);
      if (p.conflicts.length) head.appendChild(chip(p.conflicts.length + " sync conflicts", "warn", p.conflicts.join("\n")));
      const add = ibtn("plus", "", "ghost newt");
      add.title = "new task or tip in " + p.key + (data.projects.length === 1 || state.proj === p.key ? " (n)" : "");
      add.setAttribute("aria-label", add.title);
      add.addEventListener("click", () => newTask(p.key));
      head.appendChild(add);
      head.appendChild(dirDot(p, p.key, "last"));
      sec.appendChild(head);

      // Tasks | Tips tabs when the project has both.
      const hasTasks = p.tasks.length > 0;
      let tab = state.tab[p.key] || "tasks";
      if (!ptips.length || !hasTasks) tab = hasTasks ? "tasks" : "tips";
      if (q && tab === "tasks" && !tasks.length) tab = "tips";
      if (q && tab === "tips" && !ptips.length) tab = "tasks";
      if (hasTasks && ptips.length) {
        const tabs = el("div", "seg ptabs");
        tabs.setAttribute("role", "tablist");
        [["tasks", "Tasks", act.length], ["tips", "Tips", ptips.length]].forEach(([k, label, n]) => {
          const b = el("button", "", label);
          b.appendChild(el("span", "count " + k, String(n)));
          b.setAttribute("role", "tab");
          b.setAttribute("aria-selected", String(k === tab));
          b.addEventListener("click", () => { state.tab[p.key] = k; render(); });
          tabs.appendChild(b);
        });
        sec.appendChild(tabs);
      }
      if (tab === "tips" && ptips.length) {
        sec.appendChild(items(tipsList(ptips, projectWho(p.key), h)));
      } else {
        if (act.length) sec.appendChild(items(cards(tree, true)));
        else sec.appendChild(el("div", "none", "No active tasks."));
        if (arch.length) {
          const open = !!q || state.archOpen.includes(p.key);
          const tog = ibtn("down", "Archived (" + arch.length + ")", "sub" + (open ? "" : " shut"));
          tog.title = "tasks finished with Done or /handoff @task done";
          tog.addEventListener("click", () => {
            const i = state.archOpen.indexOf(p.key);
            if (i >= 0) state.archOpen.splice(i, 1); else state.archOpen.push(p.key);
            render();
          });
          sec.appendChild(tog);
          if (open) sec.appendChild(items(cards(arch)));
        }
      }
      return sec;
    };
    // Global tips: a heading and a grid of cards; folds in All, always open on its own.
    const globalSection = () => {
      const alone = view === GLOBAL;
      const open = alone || state.globalOpen || !!q;
      const h = 200;
      const sec = el("section", "globalsec");
      sec.style.setProperty("--h", h);
      const head = el("h2", "sechead" + (alone ? "" : " gtoggle" + (open ? "" : " shut")));
      const av = el("span", "avatar");
      av.appendChild(icon("globe"));
      head.appendChild(av);
      const pt = el("span", "ptitle");
      const pn = el("span", "pname", "Global tips");
      pn.appendChild(el("span", "count tips", String(gtips.length)));
      pt.appendChild(pn);
      pt.appendChild(el("span", "ppath", "hold in every project"));
      head.appendChild(pt);
      if (!alone) {
        head.tabIndex = 0;
        head.setAttribute("role", "button");
        head.setAttribute("aria-expanded", String(open));
        const toggle = () => { state.globalOpen = !state.globalOpen; render(); };
        head.addEventListener("click", toggle);
        head.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggle(); } });
        const chev = el("span", "chev");
        chev.appendChild(icon("down"));
        head.appendChild(chev);
      }
      sec.appendChild(head);
      if (open) {
        const grid = el("div", "tipgrid");
        tipsList(gtips, globalWho, h).forEach((c) => { c.classList.add("tcard"); grid.appendChild(c); });
        sec.appendChild(grid);
      }
      return sec;
    };

    if (view === GLOBAL) list.appendChild(globalSection());
    else if (view !== "all") list.appendChild(projectSection(rows.find((r) => r.p.key === view)));
    else {
      rows.forEach((r) => list.appendChild(projectSection(r)));
      if (gtips.length) list.appendChild(globalSection());
    }
    if (!rows.length && !gtips.length) {
      const any = data.projects.length || data.global.length;
      const e = el("div", "empty", any ? "Nothing matches the search." : "No handoffs yet in " + tilde(data.root) + ". Save one with /handoff or baton save.");
      if (q) {
        const clear = el("button", "", "Clear search");
        clear.addEventListener("click", clearSearch);
        e.appendChild(el("br"));
        e.appendChild(clear);
      }
      list.appendChild(e);
    }
    markOpen();
  }

  // ---- loading ------------------------------------------------------------

  async function load() {
    try {
      const list = await api("/api/projects");
      const [details, allTips] = await Promise.all([
        Promise.all(list.projects.map((p) => api("/api/projects/" + enc(p.key)))),
        api("/api/tips"),
      ]);
      const sortTasks = (ts) => ts.slice().sort((a, b) => (b.created > a.created ? 1 : -1));
      const next = {
        root: list.root,
        home: list.home || "",
        projects: list.projects.map((p, i) => {
          const d = details[i];
          const tasks = sortTasks(d.tasks.concat(d.archived)).map((t) => Object.assign({ project: p.key }, t));
          return { key: p.key, dir: d.dir || "", repo: p.repo || "", updated: p.updated, conflicts: d.conflicts, tasks, tips: d.tips };
        }),
        global: allTips.filter((t) => t.scope === GLOBAL),
      };
      $("#error").hidden = true;
      const same = data && JSON.stringify(next) === JSON.stringify(data);
      data = next;
      if (!same) render();
    } catch (e) {
      $("#error").textContent = "Cannot reach the baton dashboard (" + e.message + "). Is it still running?";
      $("#error").hidden = false;
    }
  }

  // Theme button: auto (system) -> light -> dark -> auto; kept in localStorage.
  const THEMES = { auto: "light", light: "dark", dark: "auto" };
  function setTheme(t) {
    const b = $("#theme");
    if (t === "auto") delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = t;
    try { if (t === "auto") localStorage.removeItem("baton.theme"); else localStorage.setItem("baton.theme", t); } catch (e) { /* private mode */ }
    b.textContent = "";
    b.appendChild(icon(t === "light" ? "sun" : t === "dark" ? "moon" : "auto"));
    b.title = "Theme: " + t + " (click: " + THEMES[t] + ")";
    b.setAttribute("aria-label", b.title);
  }
  setTheme(document.documentElement.dataset.theme || "auto");
  $("#theme").addEventListener("click", () => setTheme(THEMES[document.documentElement.dataset.theme || "auto"]));

  $("#reload").appendChild(icon("reload"));
  $("#reload").addEventListener("click", load);
  $("#logout").appendChild(icon("logout"));
  $("#logout").addEventListener("click", async () => {
    try { await api("/api/logout", { method: "POST" }); } catch (e) { /* signed out already */ }
    location.replace("/login");
  });
  $("#pclose").appendChild(icon("close"));
  $("#pclose").addEventListener("click", closePanel);
  $("#scrim").addEventListener("click", closePanel);
  $("#navsel").addEventListener("change", (e) => pick(e.target.value));
  window.addEventListener("popstate", () => { state.proj = fromHash(); remember(state.proj); render(); });
  $("#home").addEventListener("click", (e) => { e.preventDefault(); pick("all"); });
  state.proj = fromHash();
  if (!location.hash) {
    let v = null;
    try { v = localStorage.getItem("baton.proj"); } catch (e) { /* private mode */ }
    // A project that is gone falls back to All in render().
    if (v) { state.proj = v; history.replaceState(null, "", "#" + (v === GLOBAL ? "global-tips" : enc(v))); }
  }

  let searchTimer = 0;
  $("#q").addEventListener("input", (e) => {
    state.q = e.target.value;
    clearTimeout(searchTimer);
    if (state.q.trim().length < 2) { hits = new Map(); tipHits = new Set(); render(); return; }
    render();
    searchTimer = setTimeout(async () => {
      const q = state.q;
      try {
        const res = await api("/api/search?q=" + enc(q.trim()));
        if (q !== state.q) return;
        hits = new Map(res.tasks.map((h) => [h.project + "|" + h.task.task, h.snippet]));
        tipHits = new Set(res.tips.map(tipKey));
        render();
      } catch (err) { /* keep the local filter */ }
    }, 300);
  });
  function clearSearch() {
    $("#q").value = "";
    $("#q").dispatchEvent(new Event("input"));
  }
  // "/" focuses the search, Esc in it clears and leaves it.
  document.addEventListener("keydown", (e) => {
    const t = e.target;
    if (e.key === "Escape" && !$("#panel").hidden) { e.preventDefault(); closePanel(); return; }
    if (t === $("#q")) {
      if (e.key === "Escape") { e.preventDefault(); clearSearch(); t.blur(); }
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey || t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
    if (e.key === "/") { e.preventDefault(); $("#q").focus(); return; }
    // n: new task in the picked project (or the only one); e/f: Edit/Fork in the open panel.
    if (e.key === "n" && data) {
      const p = projectOf(state.proj) || (data.projects.length === 1 ? data.projects[0] : null);
      if (p) { e.preventDefault(); newTask(p.key); }
      return;
    }
    const b = /^[a-z]$/.test(e.key) && !$("#panel").hidden && document.querySelector('#pbody [data-hotkey="' + e.key + '"]');
    if (b && !b.disabled) { e.preventDefault(); b.click(); }
  });
  document.addEventListener("visibilitychange", () => { if (!document.hidden && session) load(); });
  setInterval(() => { if (!document.hidden && session) load(); }, 30000);

  // Installable app (needs https or localhost); the dashboard works without it.
  if ("serviceWorker" in navigator) navigator.serviceWorker.register("/sw.js").catch(() => {});

  (async function init() {
    try {
      const res = await fetch("/api/session", { credentials: "same-origin", cache: "no-store" });
      const s = await res.json();
      if (!res.ok) throw new Error(s.error || "HTTP " + res.status);
      if (s.login_required) {
        location.replace("/login");
        return;
      }
      session = s;
      $("#logout").hidden = !!s.local;
      await load();
    } catch (e) {
      $("#list").textContent = "";
      $("#list").appendChild(el("div", "error", String(e.message || e)));
    }
  })();
})();
