# Changelog

## 0.1.10

- A handoff saved in the home directory goes to the project `home`, not
  to the user name (`vova`, `root`, ...).
- Dashboard: Copy in a task panel puts the handoff Markdown on the
  clipboard (key `c`); the panel actions form one button group. The
  card button that copies `baton pickup` is now labelled Pickup.

## 0.1.9

- The dashboard writes: `+` in a project header starts a task with a short
  form (title, a name suggested from it, goal, first step) or the full
  editor, or a tip; a task panel has Edit (Markdown with preview and size
  budget), Fork and, per history entry, Restore. Saves go through the same
  store as `baton save` and are marked `by: dashboard`; a file changed
  meanwhile by an agent or a sync is not overwritten.
- Tips in the dashboard: Verified, Refuted (with the reason), Edit with a
  diff before saving, and new tips with `origin: user`.
- Next steps are a checklist (`1. [ ]`); ticking a step in the dashboard
  rewrites the current handoff in place. `/handoff` drops ticked steps,
  `/pickup` starts at the first unticked one.
- Tips are listed newest first (live ones before superseded and refuted) in
  `baton tips list` and the dashboard; before, verified ones came first and
  same-day tips had no order.
- `@task` and `[[name]]` in handoffs and tips link to tasks and tips; keys
  `n`, `e`, `f`; the picked project is remembered; a repeated `# Title` at
  the top of a handoff is not shown twice.
- The dashboard is an installable app (manifest, icons, service worker).

## 0.1.8

- `baton update` from a baton built on Termux (`android/arm64`) downloads
  the `linux_arm64` release instead of failing.
- Shorter handoff template: Goal, State, Next steps (required), then the
  optional Decisions, Context (was Key context and Gotchas), Verify; User
  preferences is gone (stable ones become tips with `origin: user`).
- `baton save` warns about a body over 3000 characters or lines over 200;
  `tips new` about a title over 70 characters or more than 8 keywords.
- `/handoff` asks for a glanceable handoff: current state instead of the
  session story, nothing that git, CLAUDE.md, README or tips already hold.

## 0.1.7

- `obsidian.vault` and `obsidian.folder` (default `baton`) put the root into
  an Obsidian vault as `<vault>/<folder>`; setting both `root` and
  `obsidian.vault` is an error.
- With `obsidian.vault`, the dashboard task and tip panels get an Obsidian
  button (`obsidian://open` link), also in `baton show --json`.
- Frontmatter edited in Obsidian keeps working: block lists (`- a` lines)
  are read, and fields baton does not know (`tags`, `aliases`, nested maps,
  `|` blocks) are kept as written when `save` or a tip status change
  rewrites the file. Before, a block list failed to parse and `save` dropped
  foreign fields.
- Values starting with a YAML indicator (`{width=`, `-Dscreen`, the `-`
  placeholder in `source`) are written quoted, so Obsidian can read the
  properties of every file baton writes.

## 0.1.6

- `baton pickup PROJECT @task "PROMPT"` passes extra instructions to the
  agent (`{prompt}` placeholder, or appended after `{task}`); the pickup
  skill and command follow them after loading the handoff.

## 0.1.5

- Dashboard: a project that is a git repository shows a GitHub/GitLab/git
  icon next to its name linking to the `origin` remote (address in the
  tooltip).

## 0.1.4

- Dashboard: the "baton" title links to All.
- Dashboard: the project header no longer repeats the last handoff age
  (each task shows its own).

## 0.1.3

- Dashboard: project availability on this machine is a green/orange dot at
  the end of the project header instead of a "no directory" chip and
  subtitle.
- Dashboard: a changed `dashboard.public_url` applies without a restart
  (requests to the new host got 421 "unknown Host header"); the old host
  stops being accepted.

## 0.1.2

- Dashboard: Markdown with `**bold**` or `*em*` no longer hangs the page
  (tip and handoff panels stayed at "Loading…").
- Dashboard: bare tip dates show "today"/"Nd ago" instead of hours since UTC
  midnight.
- Dashboard: `*em*` inside `**bold**` renders.
- Dashboard: tip ages use the file time ("5m ago"), like handoffs.
- Dashboard: every history row has View and Diff (Diff is disabled on the
  first version).
- `baton dashboard --background` fails when the port is already taken (e.g.
  by `baton service`) instead of reporting a dashboard that exited.
- Dashboard: fixed a data race on allowed host names during config reload
  (could crash the server); CSRF token compared in constant time; read and
  write timeouts; the login-failure table no longer grows without bound.
- Windows: `baton pickup` refuses to pass arguments with cmd.exe
  metacharacters to a `.cmd`/`.bat` agent shim (BatBadBut).
- systemd unit: `%` and `$` in paths are escaped.
- A `commit` value from a handoff is used with git only if it looks like a
  hash.
- Task names and project keys that are Windows device names (`con`, `nul`,
  `com1`, ...) are refused.
- Atomic writes sync the directory after the rename and replace a stale
  temp file.

## 0.1.1

- Dashboard: redesigned after claude-handoff: project sidebar, project cards
  with fork trees, side panel for tasks and tips, light/dark theme switch.
- Dashboard: pickup buttons only for `agent.*` lines in the config.
- Dashboard: empty lists no longer break the project page (`null` in JSON).
- `baton config init|path`; the install scripts write a commented default
  config.
- `baton integrate NAME` adds `agent.NAME` to the config; `--uninstall`
  removes it unless edited.

## 0.1.0

First release.

- Store: named tasks per project, history with keep-N, done/restore, rename
  (updates forks), forks, sync-conflict detection, atomic writes, per-project
  lock, `baton template`.
- Git: branch/commit in handoffs; staleness report (new commits,
  rebased/missing commit, changed files, dirty tree).
- Paths and pickup: automatic `paths`, `path set|list|prune`, agent
  templates, `pickup --print`; `exec` on Unix, child process on Windows.
- Tips: scored search with simple word forms and `--error` mode; statuses,
  supersede, move between project and global; tip suggestions in `show`.
- Dashboard: project grid, task panel with history and diff, Done/Restore/
  Rename, tip deletion, search, pickup buttons per agent; scrypt password,
  sessions with idle/max expiry, login lockout, Host/Origin checks, CSRF.
- Service: systemd `--user`, launchd, Task Scheduler; `--background/--stop`.
- Integrations: Claude Code skills and opencode commands via
  `baton integrate`.
- `baton update` from GitHub Releases with SHA256 verification;
  `install.sh`, `install.ps1`.
