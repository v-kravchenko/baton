# Changelog

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
