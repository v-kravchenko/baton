---
name: handoff
description: Save this session's state as a baton handoff for a named task so a later session on any machine or agent can continue it. Use on /handoff, when the user asks to save or hand off the session, or before a long session ends. Also "/handoff @task done" (finish a task) and "/handoff fork NEW" (split off a new task).
argument-hint: "[@task] [done|fork NEW] [notes]"
allowed-tools: Bash(baton *)
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Current tasks of this project:
!`baton tasks`

All state lives behind the `baton` CLI. Run it from the project root as is,
without `cd`. Never write files under the baton root yourself: pass bodies to
`baton save` and `baton tips new` on stdin.

## 1. Pick the task

- If the arguments name `@task`, use it.
- Otherwise use the task this session worked on (it was picked up or is listed
  above). With no tasks yet, choose a short kebab-case name for the work, such
  as `@api-refactor`. If several tasks could fit, ask the user.

## 2. Read the previous handoff

Run `baton show @task`. Keep only what the next steps still need. Drop what
this session finished and what git, CLAUDE.md, README or tips now hold.
History keeps only the last few versions (`keep`, 10 by default), so an open
decision or parked item must stay in the handoff or move to a tip.

## 3. Write the handoff

Run `baton template` for the sections. Goal, State and Next steps are
required; skip the others when they have nothing to say.

The reader is the next agent and the user scanning it. Write for a glance:

- Budget: about 3000 characters. One fact per bullet, under 200 characters,
  at most one level of nesting.
- State is where things stand now, not the story of this session.
- `path:line` instead of code; the exact command or error text only when it
  is the point.
- Do not repeat what lives elsewhere: commits (git log), project rules
  (CLAUDE.md, README), tips (name the tip ID instead). A stable user
  preference that is not specific to this task becomes a tip with
  `origin: user`, not a handoff line.

Good: `- update: TLS fails on Termux, fix in internal/update/http.go:40 (not started)`
Bad: `- This session we looked into update and found that on Termux, because the
  static binary does not know the CA path, TLS fails, so we decided ...`

Save it with a quoted heredoc:

```bash
baton save @task --title "Short title" <<'HANDOFF'
## Goal
...
HANDOFF
```

baton adds frontmatter, git branch/commit, history and the project path. If it
warns about missing sections or the size budget, fix the body and save again.

## 4. Tips

For each non-obvious trap of this session that would help other sessions (a
misleading error, a flag that does not work, an environment quirk):

1. `baton tips search WORDS` to find duplicates.
2. If an existing tip is wrong or outdated, write the new one and run
   `baton tips supersede OLD NEW`; if this session confirmed one,
   `baton tips verified ID`.
3. Otherwise add it (`--global` when it is not specific to this project). One
   fact per tip, title under 70 characters, up to 8 keywords an agent would
   search for:

```bash
baton tips new <<'TIP'
---
title: One-line summary
when: situation in which it applies
keywords: [word, other-word]
origin: session
---
Tip: what to do.
Why: what happens otherwise and why.
Verify: a quick command or check that proves it still holds.
TIP
```

`origin` is `session` (found here), `failure` (an error hit), `web` (docs) or
`user` (the user said so). Only add tips worth keeping; most sessions add none.

## Modes

- `/handoff @task done`: save a final handoff if the session changed anything,
  then `baton done task`.
- `/handoff fork NEW`: first save the current task (`baton save @parent`), then
  save the forked work with `baton save @NEW --from parent`.

Finish by telling the user the task name and that `/pickup @task` (or
`baton pickup <project> @task` from any directory) continues it.
