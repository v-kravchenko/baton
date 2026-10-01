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
---
## Goal
...
```

There are no machine paths, host names, task names (the file name is the task)
or agent session ids. Unknown fields are ignored. Only `baton save` writes
frontmatter; agents pass the body on stdin and `--title`. The body follows the
template printed by `baton template` (Goal, State, Decisions, Key context,
Gotchas, User preferences, Next steps, Verify); `save` warns about missing
sections. Markdown is read with `\r\n` or `\n` and always written with `\n`.

### Tips

Tip frontmatter: `title`, `when`, `keywords`, `cites` (`file@commit`),
`origin` (`session`/`failure`/`web`), `source` (`[project, commit, date]`),
`status` (`active`/`verified`/`refuted`/`superseded`), `env` (`termux`,
`darwin`, ...). Body: `Tip:`, `Why:`, `Verify:`.

Search scores keywords > title > when > body, matches simple word forms
(English and Ukrainian endings, prefixes), hides refuted and superseded tips
(`--all` shows them) and ranks verified tips higher. `--error` treats the
query as error output: numbers and hashes are dropped and at least two terms
must match.

There are no agent hooks. Tips surface in two ways: `baton show` lists tips
matching the task name, title and Gotchas section (titles only), and the
installed instruction block plus the `/tips` skill tell the agent to run
`baton tips search` before debugging.

## CLI

The project directory is the current directory or `--dir`. Every read command
accepts `--json`. Report commands (`show`, `tasks`, `stale`, `tips search`)
exit 0 on states such as "no handoff" and describe the state in the output;
a non-zero exit means a real error.

```text
baton save [@task] [--from PARENT] [--title T]   # body on stdin
baton show [@task|FILE]                          # handoff + staleness + forks + tips, or a STATE line
baton tasks | done TASK | restore TASK | rename OLD NEW | history TASK [N] | stale FILE|@task
baton pickup [PROJECT] [@task] [--agent X] [--print]
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

An agent template is split into arguments on whitespace, honoring `"..."` and
`'...'`. Backslash is not an escape character, so Windows paths
(`C:\tools\x.exe`) work as is. Placeholders are substituted in each argument
separately and the program runs without a shell, so there is no injection:
`{task}` (`@name` or empty; trailing spaces of the argument are trimmed, so
`"/pickup {task}"` becomes `/pickup`), `{project}`, `{dir}`, `{root}`.
The dashboard shows a copy-pickup button per `agent.*` line, in their order;
without `agent.*` lines it shows none (the built-in agents serve the CLI only).

## Dashboard

`baton dashboard` serves the UI on `dashboard.host:dashboard.port` and opens a
browser (`xdg-open`, `open`, `termux-open-url`, `wslview` or `rundll32`;
otherwise it prints the URL). `--background` detaches it (log in the state
directory) and `--stop` stops it. `baton service install` runs it at login via
systemd `--user`, launchd or Task Scheduler; Termux has no service manager,
use `--background`.

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
