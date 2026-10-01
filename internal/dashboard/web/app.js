"use strict";
(function () {
  const view = document.getElementById("view");
  const toastEl = document.getElementById("toast");
  let session = null;

  // ---- helpers ----------------------------------------------------------

  function el(tag, attrs, ...children) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v === undefined || v === null || v === false) continue;
      if (k === "class") n.className = v;
      else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
      else n.setAttribute(k, v === true ? "" : v);
    }
    for (const c of children.flat()) {
      if (c === null || c === undefined || c === false) continue;
      n.appendChild(typeof c === "string" || typeof c === "number" ? document.createTextNode(String(c)) : c);
    }
    return n;
  }

  function toast(msg, isError) {
    toastEl.textContent = msg;
    toastEl.className = isError ? "error" : "";
    toastEl.hidden = false;
    clearTimeout(toast.t);
    toast.t = setTimeout(() => (toastEl.hidden = true), 3500);
  }

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
    });
    if (res.status === 401) {
      location.replace("/login");
      throw new Error("login required");
    }
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || res.statusText);
    return data;
  }

  const enc = encodeURIComponent;

  function ago(iso) {
    const t = new Date(iso).getTime();
    if (!t) return "";
    const s = (Date.now() - t) / 1000;
    if (s < 60) return "just now";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 172800) return Math.floor(s / 3600) + "h ago";
    return Math.floor(s / 86400) + "d ago";
  }

  function when(iso) {
    const d = new Date(iso);
    return isNaN(d) ? "" : d.toLocaleString();
  }

  function gitText(h) {
    if (h.branch && h.commit) return h.branch + " @ " + h.commit;
    return h.commit ? "@ " + h.commit : h.branch || "";
  }

  async function copy(text) {
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
      } else {
        const ta = el("textarea", { readonly: true, class: "offscreen" });
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand("copy");
        ta.remove();
      }
      toast("Copied: " + text);
    } catch (e) {
      toast("Copy failed; select the command by hand", true);
    }
  }

  function crumbs(...parts) {
    const nav = el("nav", { class: "crumbs" }, el("a", { href: "#/" }, "projects"));
    for (const [label, href] of parts) {
      nav.appendChild(document.createTextNode(" / "));
      nav.appendChild(href ? el("a", { href }, label) : el("span", null, label));
    }
    return nav;
  }

  function badge(text, cls) {
    return el("span", { class: "badge " + (cls || "") }, text);
  }

  function show(...nodes) {
    view.replaceChildren(...nodes);
    window.scrollTo(0, 0);
  }

  function fail(e) {
    show(el("p", { class: "error" }, String(e.message || e)));
  }

  // ---- views ------------------------------------------------------------

  async function viewProjects() {
    const data = await api("/api/projects");
    const grid = el("div", { class: "grid" });
    for (const p of data.projects) {
      const tasks = el("ul", { class: "tasks" });
      for (const t of p.tasks.slice(0, 5)) {
        tasks.appendChild(
          el("li", null,
            el("a", { href: `#/p/${enc(p.key)}/t/${enc(t.task)}` }, "@" + t.task),
            " ", el("span", { class: "title" }, t.title), " ",
            el("span", { class: "muted" }, ago(t.created))));
      }
      if (p.tasks.length > 5) tasks.appendChild(el("li", { class: "muted" }, `+${p.tasks.length - 5} more`));
      if (!p.tasks.length) tasks.appendChild(el("li", { class: "muted" }, "no active tasks"));
      grid.appendChild(
        el("section", { class: "card project" },
          el("h2", null, el("a", { href: `#/p/${enc(p.key)}` }, p.key)),
          el("div", { class: "meta" },
            p.dir ? el("code", { title: "directory on this machine" }, p.dir) : el("span", { class: "muted" }, "no directory on this machine"),
          ),
          tasks,
          el("div", { class: "meta" },
            p.archived ? badge(p.archived + " archived") : null,
            p.tips ? badge(p.tips + " tips") : null,
            p.conflicts ? badge(p.conflicts + " sync conflicts", "warn") : null,
            p.updated && !p.updated.startsWith("0001") ? el("span", { class: "muted" }, "updated " + ago(p.updated)) : null)));
    }
    grid.appendChild(
      el("section", { class: "card project global" },
        el("h2", null, el("a", { href: "#/tips" }, "tips")),
        el("p", { class: "muted" }, `${data.global_tips} global tips; all tips across projects`)));
    show(
      el("h1", null, "Projects"),
      data.projects.length ? null : el("p", { class: "muted" }, `No handoffs yet in ${data.root}. Run baton save in a project.`),
      grid);
  }

  function taskList(p, list) {
    if (!list.length) return el("p", { class: "muted" }, "none");
    return el("ul", { class: "tasks" }, list.map((t) =>
      el("li", null,
        el("a", { href: `#/p/${enc(p)}/t/${enc(t.task)}` }, "@" + t.task), " ",
        el("span", { class: "title" }, t.title), " ",
        el("span", { class: "muted" }, ago(t.created) + (gitText(t) ? " · " + gitText(t) : "")),
        t.from ? el("span", { class: "muted" }, " · fork of @" + t.from) : null)));
  }

  function tipList(list) {
    if (!list.length) return el("p", { class: "muted" }, "none");
    return el("ul", { class: "tips" }, list.map((t) =>
      el("li", null,
        el("a", { href: `#/tip/${enc(t.scope)}/${enc(t.id)}` }, t.title), " ",
        t.status !== "active" ? badge(t.status, t.status) : null, " ",
        el("span", { class: "muted" }, t.scope + (t.when ? " · when " + t.when : "")))));
  }

  async function viewProject(p) {
    const d = await api(`/api/projects/${enc(p)}`);
    show(
      crumbs([p]),
      el("h1", null, p),
      d.dir ? el("p", null, el("code", null, d.dir)) : el("p", { class: "muted" }, "No directory on this machine (baton path set " + p + " DIR)"),
      d.conflicts.length ? el("div", { class: "card warn" }, el("strong", null, "Sync conflicts"), el("ul", null, d.conflicts.map((c) => el("li", null, el("code", null, c))))) : null,
      el("h2", null, "Tasks"), taskList(p, d.tasks),
      el("h2", null, "Archived"), taskList(p, d.archived),
      el("h2", null, "Tips"), tipList(d.tips));
  }

  async function viewTask(p, t) {
    const d = await api(`/api/projects/${enc(p)}/tasks/${enc(t)}`);
    const h = d.handoff;
    const bodyBox = el("div", { class: "card body" }, window.renderMarkdown(d.body));
    const bodyTitle = el("h2", null, "Current handoff");

    const actions = el("div", { class: "actions" });
    if (h.archived) {
      actions.appendChild(el("button", { onclick: () => act("restore") }, "Restore"));
    } else {
      actions.appendChild(el("button", { onclick: () => act("done") }, "Done"));
    }
    actions.appendChild(el("button", { class: "ghost", onclick: rename }, "Rename"));

    async function act(what) {
      try {
        await api(`/api/projects/${enc(p)}/tasks/${enc(t)}/${what}`, { method: "POST" });
        toast(what === "done" ? "Archived @" + t : "Restored @" + t);
        route();
      } catch (e) {
        toast(e.message, true);
      }
    }

    async function rename() {
      const name = prompt("New name for @" + t, t);
      if (!name || name === t) return;
      try {
        const r = await api(`/api/projects/${enc(p)}/tasks/${enc(t)}/rename`, { method: "POST", body: { name } });
        toast("Renamed to @" + r.task);
        location.hash = `#/p/${enc(p)}/t/${enc(r.task)}`;
      } catch (e) {
        toast(e.message, true);
      }
    }

    const pickup = el("div", { class: "pickup" },
      d.pickup.map((pk) =>
        el("button", { class: "pick", title: pk.command, onclick: () => copy(pk.command) }, "pickup " + pk.agent)));

    async function showVersion(n, label) {
      try {
        const v = await api(`/api/projects/${enc(p)}/tasks/${enc(t)}/history/${n}`);
        bodyTitle.textContent = label;
        bodyBox.replaceChildren(window.renderMarkdown(v.body));
        bodyBox.scrollIntoView({ behavior: "smooth" });
      } catch (e) {
        toast(e.message, true);
      }
    }

    async function showDiff(n) {
      try {
        const v = await api(`/api/projects/${enc(p)}/tasks/${enc(t)}/diff?from=${n}&to=0`);
        bodyTitle.textContent = `Diff: history #${n} → current`;
        const pre = el("pre", { class: "diff" }, v.lines.map((l) =>
          el("span", { class: l.op === "+" ? "add" : l.op === "-" ? "del" : "" }, l.op + " " + l.text + "\n")));
        bodyBox.replaceChildren(pre);
        bodyBox.scrollIntoView({ behavior: "smooth" });
      } catch (e) {
        toast(e.message, true);
      }
    }

    const history = d.history.length
      ? el("ol", { class: "history" }, d.history.map((x, i) =>
        el("li", null,
          el("span", null, when(x.created)), " ",
          el("span", { class: "title" }, x.title), " ",
          el("span", { class: "muted" }, gitText(x)), " ",
          el("button", { class: "ghost small", onclick: () => showVersion(i + 1, `History #${i + 1}: ${when(x.created)}`) }, "view"),
          el("button", { class: "ghost small", onclick: () => showDiff(i + 1) }, "diff"))))
      : el("p", { class: "muted" }, "no history");

    const stale = d.staleness_lines && d.staleness_lines.length
      ? el("div", { class: "card stale" }, el("strong", null, "Staleness"), el("pre", null, d.staleness_lines.join("\n")))
      : null;

    show(
      crumbs([p, `#/p/${enc(p)}`], ["@" + t]),
      el("h1", null, h.title, " ", h.archived ? badge("archived") : null),
      el("p", { class: "meta" },
        el("span", null, "@" + h.task), " · ",
        el("span", { title: when(h.created) }, "saved " + ago(h.created)),
        gitText(h) ? [" · ", el("code", null, gitText(h))] : null,
        h.from ? [" · fork of ", el("a", { href: `#/p/${enc(p)}/t/${enc(h.from)}` }, "@" + h.from)] : null,
        d.forks && d.forks.length ? [" · forks: ", d.forks.map((f, i) => [i ? ", " : "", el("a", { href: `#/p/${enc(p)}/t/${enc(f)}` }, "@" + f)])] : null),
      el("div", { class: "toolbar" }, pickup, actions),
      stale,
      bodyTitle,
      bodyBox,
      el("h2", null, "History"),
      history);
    bodyTitle.addEventListener("dblclick", () => {
      bodyTitle.textContent = "Current handoff";
      bodyBox.replaceChildren(window.renderMarkdown(d.body));
    });
  }

  async function viewTips() {
    const list = await api("/api/tips");
    show(crumbs(["tips"]), el("h1", null, "Tips"), tipList(list));
  }

  async function viewTip(scope, id) {
    const d = await api(`/api/tips/${enc(scope)}/${enc(id)}`);
    const t = d.tip;
    async function del() {
      if (!confirm(`Delete tip "${t.title}"? This removes the file.`)) return;
      try {
        await api(`/api/tips/${enc(scope)}/${enc(id)}`, { method: "DELETE" });
        toast("Deleted tip " + id);
        location.hash = scope === "global" ? "#/tips" : `#/p/${enc(scope)}`;
      } catch (e) {
        toast(e.message, true);
      }
    }
    const meta = [
      ["id", t.id], ["scope", t.scope], ["status", t.status], ["when", t.when],
      ["keywords", (t.keywords || []).join(", ")], ["cites", (t.cites || []).join(", ")],
      ["env", (t.env || []).join(", ")], ["origin", t.origin], ["source", (t.source || []).join(" ")],
      ["verified", d.verified], ["superseded by", d.superseded_by],
    ].filter(([, v]) => v);
    show(
      crumbs(scope === "global" ? ["tips", "#/tips"] : [scope, `#/p/${enc(scope)}`], [t.title]),
      el("h1", null, t.title, " ", t.status !== "active" ? badge(t.status, t.status) : null),
      el("dl", { class: "kv" }, meta.map(([k, v]) => [el("dt", null, k), el("dd", null, v)])),
      el("div", { class: "card body" }, window.renderMarkdown(t.body)),
      el("div", { class: "actions" }, el("button", { class: "danger", onclick: del }, "Delete tip")));
  }

  async function viewSearch(q) {
    document.getElementById("q").value = q;
    const d = await api("/api/search?q=" + enc(q));
    show(
      crumbs(["search"]),
      el("h1", null, "Search: " + q),
      el("h2", null, "Tasks"),
      d.tasks.length
        ? el("ul", { class: "tasks" }, d.tasks.map((h) =>
          el("li", null,
            el("a", { href: `#/p/${enc(h.project)}/t/${enc(h.task.task)}` }, h.project + " @" + h.task.task), " ",
            el("span", { class: "title" }, h.task.title), " ",
            h.task.archived ? badge("archived") : null,
            h.snippet ? el("div", { class: "muted snippet" }, h.snippet) : null)))
        : el("p", { class: "muted" }, "no tasks"),
      el("h2", null, "Tips"),
      tipList(d.tips));
  }

  // ---- routing ----------------------------------------------------------

  async function route() {
    const hash = location.hash.replace(/^#/, "") || "/";
    const [path, query] = hash.split("?");
    const parts = path.split("/").filter(Boolean).map(decodeURIComponent);
    try {
      if (parts.length === 0) await viewProjects();
      else if (parts[0] === "p" && parts.length === 2) await viewProject(parts[1]);
      else if (parts[0] === "p" && parts[2] === "t" && parts.length === 4) await viewTask(parts[1], parts[3]);
      else if (parts[0] === "tips") await viewTips();
      else if (parts[0] === "tip" && parts.length === 3) await viewTip(parts[1], parts[2]);
      else if (parts[0] === "search") await viewSearch(new URLSearchParams(query || "").get("q") || "");
      else show(el("p", null, "Not found. ", el("a", { href: "#/" }, "Projects")));
    } catch (e) {
      fail(e);
    }
  }

  document.getElementById("search").addEventListener("submit", (e) => {
    e.preventDefault();
    const q = document.getElementById("q").value.trim();
    if (q) location.hash = "#/search?q=" + enc(q);
  });

  document.getElementById("logout").addEventListener("click", async () => {
    await fetch("/api/logout", { method: "POST", headers: { "X-Baton-Token": session.csrf }, credentials: "same-origin" });
    location.replace("/login");
  });

  window.addEventListener("hashchange", route);

  (async function init() {
    try {
      const res = await fetch("/api/session", { credentials: "same-origin" });
      session = await res.json();
      if (!res.ok) throw new Error(session.error || res.statusText);
      if (session.login_required) {
        location.replace("/login");
        return;
      }
      document.getElementById("logout").hidden = !!session.local;
      route();
    } catch (e) {
      fail(e);
    }
  })();
})();
