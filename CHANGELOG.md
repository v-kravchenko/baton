# Changelog

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
