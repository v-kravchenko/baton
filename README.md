# baton

[![ci](https://github.com/v-kravchenko/baton/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/v-kravchenko/baton/actions/workflows/ci.yml)
[![release](https://github.com/v-kravchenko/baton/actions/workflows/release.yml/badge.svg)](https://github.com/v-kravchenko/baton/actions/workflows/release.yml)
[![latest release](https://img.shields.io/github/v/release/v-kravchenko/baton?sort=semver)](https://github.com/v-kravchenko/baton/releases/latest)
[![go version](https://img.shields.io/github/go-mod/go-version/v-kravchenko/baton)](go.mod)
[![license](https://img.shields.io/github/license/v-kravchenko/baton)](LICENSE)

`baton` is a standalone CLI that lets a coding agent (Claude Code, opencode or
any other) save the state of a session as a Markdown handoff for a named task,
so that a later session on any machine and with any agent can pick it up.

- **Tasks.** Several named tasks per project (`@api-refactor`), history with
  keep-N, done/restore, rename, forks (`from:`), staleness report from git.
- **Pickup.** `baton pickup <project> [@task]` changes to the project directory
  on this machine and starts the configured agent with a resume prompt.
- **Tips.** Short unverified hints with a Verify step; statuses
  verified/refuted/superseded; project and global level; keyword search.
- **Dashboard.** Project grid, task panel with history and diff,
  Done/Restore/Rename, tip deletion, search, pickup buttons per agent, password
  login for non-loopback access. Access is all or nothing: there is no
  read-only mode.
- **Sync.** The root is plain `.md` files you can share between machines
  (Syncthing etc.); per-machine state lives outside the root.

One static binary, no runtime dependencies. `git` is optional (without it there
is no branch/commit/staleness data).

## Install

```sh
# Linux, macOS, Termux
curl -fsSL https://raw.githubusercontent.com/v-kravchenko/baton/main/install.sh | sh
# with agent integrations
curl -fsSL https://raw.githubusercontent.com/v-kravchenko/baton/main/install.sh | sh -s -- --integrate claude,opencode
```

```powershell
# Windows
irm https://raw.githubusercontent.com/v-kravchenko/baton/main/install.ps1 | iex
```

Or `go install github.com/v-kravchenko/baton/cmd/baton@latest` (Go 1.22+).

The scripts verify the SHA256 from the release's `checksums.txt`. Later
updates: `baton update` (`--check` only reports).

macOS: binaries downloaded by a browser are blocked by Gatekeeper because they
are not signed; `install.sh` uses `curl` and is not affected. Windows:
SmartScreen warns about the unsigned `.exe` when it comes from a browser.

## Quick start

```sh
cd ~/src/api
baton integrate claude          # /handoff, /pickup, /tips for Claude Code
# ... in an agent session: /handoff @auth ...
baton tasks                     # @auth: Refresh token flow, 2h ago · main @ a1b2c3d
cd ~ && baton pickup api @auth  # cd ~/src/api && claude "/pickup @auth"
baton dashboard                 # http://127.0.0.1:8765/
```

## Concepts

### Project key

The project key is always the name of the project directory, lower-cased, with
every character outside `a-z0-9._-` replaced by `-`. Neither `paths` nor git
affect it. Consequences, accepted on purpose:

- two different directories with the same name (`client-a/api`,
  `client-b/api`) are one project; rename a directory to separate them;
- two clones of one repository with the same name are the same project;
- a renamed directory is a new project; move the old data with
  `mv <root>/<old> <root>/<new>`.

The home directory itself (`~`, a session started outside any project) is
the project `home` for every user name and machine, so it syncs as one.

`global` is reserved; baton refuses a project directory with that name.

### Root layout

```text
<root>/
├── <project>/
│   ├── tasks/<task>.md               # latest handoff of a task
│   ├── archive/<task>.md             # finished tasks
│   ├── history/<task>/<created>.md   # previous handoffs (keep-N)
│   └── tips/<id>.md                  # project tips
└── global/
    └── tips/<id>.md                  # global tips
```

History file names are the `created` time in UTC (`20261001T071500Z.md`).
Writes are atomic (hidden `.<name>.tmp` + rename). Changes to one project are
serialized with a lock file in the state directory (flock / LockFileEx), so two
sessions on one machine cannot corrupt history. There is no locking between
machines: Syncthing conflict files (`*.sync-conflict-*`) are reported by
`show`, `tasks` and the dashboard instead of being lost.

### Handoff format

```yaml
---
title: "Refresh token flow"
created: 2026-10-01T10:15:00+03:00
branch: main          # only in a git repository
commit: a1b2c3d       # only in a git repository
from: auth-rewrite    # only for forks: the parent task
by: dashboard         # only when saved in the dashboard
---
## Goal
...
```

There are no machine paths, host names, task names (the file name is the task)
or agent session ids. Only `baton save` and the dashboard write frontmatter; agents pass the
body on stdin and `--title`. Fields added by other tools (Obsidian `tags`,
`aliases`, ...) are ignored by baton and kept when it rewrites a handoff or a
tip. Frontmatter is flat `key: value` lines; lists are `[a, b]` or block
lists (`- a` lines under the key). Other multi-line values (nested maps,
`|` / `>` blocks) are kept as written. The body follows the
template printed by `baton template` (Goal, State, Next steps, then the
optional Decisions, Context, Verify), with Next steps as an ordered
checklist (`1. [ ]`, `[x]` once done); `save` warns about missing required
sections and about a body over 3000 characters or with lines over 200
characters (fenced code is not counted). Markdown is read with `\r\n` or `\n` and always written with `\n`.

### Tips

Tip frontmatter: `title`, `when`, `keywords`, `cites` (`file@commit`),
`origin` (`session`/`failure`/`web`/`user`), `source` (`[project, commit, date]`),
`status` (`active`/`verified`/`refuted`/`superseded`), `env` (`termux`,
`darwin`, ...). Body: `Tip:`, `Why:`, `Verify:`. `tips new` warns about a title over 70
characters or more than 8 keywords.

Search scores keywords > title > when > body, matches simple word forms
(English and Ukrainian endings, prefixes), hides refuted and superseded tips
(`--all` shows them) and ranks verified tips higher. `--error` treats the
query as error output: numbers and hashes are dropped and at least two terms
must match.

There are no agent hooks. Tips surface in two ways: `baton show` lists tips
matching the task name, title and Context (or older Gotchas) section (titles
only), and the installed instruction block plus the `/tips` skill tell the
agent to run `baton tips search` before debugging.

## CLI

The project directory is the current directory or `--dir`. Every read command
accepts `--json`. Report commands (`show`, `tasks`, `stale`, `tips search`)
exit 0 on states such as "no handoff" and describe the state in the output;
a non-zero exit means a real error.

```text
baton save [@task] [--from PARENT] [--title T]   # body on stdin
baton show [@task|FILE]                          # handoff + staleness + forks + tips, or a STATE line
baton tasks | done TASK | restore TASK | rename OLD NEW | history TASK [N] | stale FILE|@task
baton pickup [PROJECT] [@task] [PROMPT] [--agent X] [--print]
baton path set KEY DIR | list | prune
baton tips search [--error] [--all] WORDS | show ID | new [--global] | verified ID |
           refuted ID WHY | supersede OLD NEW | move ID global|project | list [--all]
baton dashboard [--port N] [--host H] [--no-open] [--background|--stop] [--json]
baton service install|status|restart|uninstall [--dry-run]
baton auth status|on|off|password|logout-all
baton integrate claude|opencode [--uninstall] [--force]
baton config init|path
baton template
baton update [--check] [--force]
baton version
```

- `save` without `@task` uses the only active task, `@main` when there is
  none, and fails when there are several. Saving an archived task makes it
  active again. The previous handoff moves to `history/`, keeping `keep`
  entries.
- Commands that read stdin (`save`, `tips new`, `tips search --error`) exit 2
  with a hint when stdin is a terminal instead of waiting for input.
- `show` states: `CHOOSE TASK` (several tasks, none given), `NO TASK` (the
  task does not exist), `ARCHIVED` (the task is done), `NO HANDOFF` (nothing
  active).
- `tips new` reads the tip on stdin; frontmatter fields may come from stdin or
  from `--title --when --keywords --cites --origin --env`.

### Paths and pickup

`~/.config/baton/paths` maps a project key to its directory on this machine.
`save` and `show` record the current directory silently: the last one used
wins. Manage it with `baton path set|list|prune`.

`baton pickup PROJECT` uses the recorded directory. If it is missing and the
current directory has the same key, it uses (and records) the current
directory. `baton pickup` without a project uses the current directory if
baton has handoffs for its key, and otherwise fails with a hint, so running it
in `~` does not look for a project called after your user name.

Agents are named templates in the config. `pickup` uses `agent.default`;
`--agent X` picks another. `--print` shows the command instead of running it.

`baton pickup PROJECT @task "PROMPT"` passes extra instructions to the agent:
they replace `{prompt}`, or follow `{task}` in the same argument when the
template has no `{prompt}`, so `claude "/pickup {task}"` becomes
`claude "/pickup @task PROMPT"`. A prompt needs PROJECT and `@task`.
On Unix baton `exec`s the agent; on Windows it starts it as a child process
with inherited stdio and returns its exit code.

## Configuration

```text
~/.config/baton/config     # everything about baton
~/.config/baton/paths      # key=path per line; per machine, not synced
~/.local/state/baton/      # password (0600), sessions, baton.pid, baton.log, locks
<root>/                    # default ~/.local/share/baton
```

Overrides: `BATON_CONFIG_DIR` (directory with `config` and `paths`),
`BATON_STATE`, `BATON_ROOT`. On Windows the same paths are relative to
`%USERPROFILE%`.

```text
root = ~/Sync/handoffs
keep = 10
obsidian.vault =
obsidian.folder = baton
dashboard.host = 127.0.0.1
dashboard.port = 8765
dashboard.public_url =
dashboard.auth = off
dashboard.auth.idle = 7d
dashboard.auth.max = 30d
agent.default = claude
agent.claude = claude "/pickup {task}"
agent.opencode = opencode --prompt "/pickup {task}"
```

Flat `key = value`; `#` starts a comment at the beginning of a line or after
whitespace. An unknown key is a warning, not an error. Without any `agent.*`
lines the two agents above are built in.

`baton config init` (run by the install scripts) writes the file with every
key commented out at its default, unless it exists; `baton config path`
prints its location. `baton integrate NAME` appends `agent.NAME` with the
template above unless a line for it exists; `--uninstall` removes it again
(and `agent.default` naming it) unless it was edited.

`obsidian.vault` (the vault directory, empty by default) turns on the
Obsidian integration and puts the root into that vault:
`<obsidian.vault>/<obsidian.folder>` (folder `baton` by default; nested
folders such as `Dev/baton` work). Set either `root` or `obsidian.vault`, not
both: with both, every command fails. The folder must be a relative path inside
the vault, not the vault itself, since baton would take every vault folder
for a project. `BATON_ROOT` still overrides both. If the new root does not
exist yet while the default root has data, commands print the `mv` that moves
it. The vault path is per machine like the rest of the config.

An agent template is split into arguments on whitespace, honoring `"..."` and
`'...'`. Backslash is not an escape character, so Windows paths
(`C:\tools\x.exe`) work as is. Placeholders are substituted in each argument
separately and the program runs without a shell, so there is no injection:
`{task}` (`@name` or empty; trailing spaces of the argument are trimmed, so
`"/pickup {task}"` becomes `/pickup`), `{prompt}` (the pickup prompt or
empty), `{project}`, `{dir}`, `{root}`.
The dashboard shows a copy-pickup button per `agent.*` line, in their order;
without `agent.*` lines it shows none (the built-in agents serve the CLI only).

## Dashboard

`baton dashboard` serves the UI on `dashboard.host:dashboard.port` and opens a
browser (`xdg-open`, `open`, `termux-open-url`, `wslview` or `rundll32`;
otherwise it prints the URL). `--background` detaches it (log in the state
directory) and `--stop` stops it. `baton service install` runs it at login via
systemd `--user`, launchd or Task Scheduler; Termux has no service manager,
use `--background`.

The dashboard edits handoffs too, saving them like `baton save` (the
previous text goes to history, `by: dashboard` marks the save, the title
follows the first `# ` heading, branch and commit come from the project
directory on this machine or stay as they were): `+` in a project header
starts a task with a short form (title, a name suggested from it, goal and a
first step; Full editor continues with the whole template), and a task panel has Edit, Fork (a new task
with `from` set, starting from the parent's text), Copy (the handoff Markdown to the
clipboard) and, per history entry,
Restore (save that version as the current one). Checklist items (`1. [ ]`
in Next steps) tick with a click; that rewrites the current handoff in
place, without a history entry or a new save time. A save carries the version
it edited; if the file changed in the meantime (an agent, a sync), it is not
written, and the editor shows the latest text, so the next Save replaces it
knowingly. Size budgets show while typing and come back as warnings, not
errors.

Tips are edited there as well: `+` offers Task or Tip (a new tip gets
`origin: user` and the project or global scope), and a tip panel has
Verified, Refuted (asks for the reason), Edit and Delete. Edit changes the
title, `when`, keywords and text, keeps the id and the other fields, and
shows the diff before saving, since tips have no history.

In a handoff or tip, `@task` links to a task of the same project and
`[[name]]` (or `[[name|label]]`, as in Obsidian) to a task or else a tip of
the project or global; unknown names stay text. Keys: `/` search, `n` new
task in the picked project, `e` Edit, `f` Fork and `c` Copy in the open panel. The
picked project is remembered in the browser. The dashboard is an installable
app (manifest and service worker; browsers allow it on https or localhost):
the page and static files work offline from the last copy, the API is never
cached.

A project header shows the project directory on this machine and, when it is
a git repository, an icon link to its `origin` remote (the first remote without
`origin`) as `https://host/path`; local-path remotes get no link.

With `obsidian.vault` set, the task and tip panels have an `obsidian` button
next to the agent buttons (`obsidian://open?vault=...&file=...`, the vault
name being its folder name)
that opens the file in the Obsidian app on the device running the browser,
and `baton show --json` has the same link in `handoff.obsidian`. The link
assumes the vault has the same name and layout there.

Access control:

- Direct loopback clients have full access without a password.
- Anyone else needs `baton auth password` and `baton auth on`; without both,
  remote clients get 403. Requests relayed by a proxy (`X-Forwarded-*`,
  `Forwarded`) or addressed to the `public_url` host are never treated as
  loopback.
- Passwords are stored as scrypt hashes. Sessions expire after
  `dashboard.auth.idle` without use or `dashboard.auth.max` in total;
  `baton auth logout-all` ends them at once. Repeated failed logins lock the
  client out with growing delays.
- The Host header must be an IP literal, `localhost`, this machine's host
  name, or the `public_url` host (DNS rebinding protection). Changing
  requests need a same-origin `Origin` and a CSRF token.

Config changes (auth, agents, `public_url`) apply to a running dashboard
without a restart; `host`, `port` and `root` need `baton service restart`.

On Windows `0600` permissions have no effect; the state directory is
protected by the ACLs of your profile.

## Integrations

`baton integrate claude` writes `~/.claude/skills/{handoff,pickup,tips}/SKILL.md`
and appends an instruction block to `~/.claude/CLAUDE.md`
(`CLAUDE_CONFIG_DIR` is honored). `baton integrate opencode` writes
`~/.config/opencode/commands/{handoff,pickup,tips}.md` and a block in
`~/.config/opencode/AGENTS.md`. Skills contain only Markdown and call `baton`
from `PATH`; Claude Code skills need nothing but `Bash(baton *)`. Files not
written by baton are never overwritten without `--force`; `--uninstall`
removes only baton's files and block. `baton update` refreshes installed
integrations.

## Platforms

Static builds (`CGO_ENABLED=0`) for `linux/amd64`, `linux/arm64` (also
Termux), `linux/arm` (`GOARM=7`), `darwin/arm64`, `darwin/amd64`,
`windows/amd64`, `windows/arm64`.

- Termux: the home directory comes from `$HOME`; `baton update` resolves DNS
  through `$PREFIX/etc/resolv.conf` because Android has no
  `/etc/resolv.conf`.
- Windows: renames are retried while Syncthing, an antivirus or an editor
  holds a file; `baton update` renames the running `.exe` aside. Claude Code
  skills run through Git Bash, so their `baton` calls are the same.

## Development

```sh
go test ./...                    # unit tests + testscript e2e (cmd/baton/testdata/script)
go vet ./... && gofmt -l .
node --check internal/dashboard/web/*.js
```

Releases: push a `v*` tag; GoReleaser publishes the binaries and
`checksums.txt`.

## License

MIT, see [LICENSE](LICENSE).
