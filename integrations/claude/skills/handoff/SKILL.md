---
name: handoff
description: Save this session's state as a baton handoff for a named task, with the task's docs, so a later session on any machine or agent can continue it. Use on /handoff, when the user asks to save or hand off the session, or before a long session ends. Also "/handoff @task done" (finish a task) and "/handoff fork NEW" (split off a new task).
argument-hint: "[@task] [done|fork NEW] [notes]"
allowed-tools: Bash(baton *)
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Current tasks of this project:
!`baton tasks`

Run `baton` from the project root as is, without `cd`. Never write files
under the baton root yourself: pass bodies on stdin.

## Commands

| Command | For |
|---|---|
| `baton show @task` | the current handoff, its docs<!-- tips --> and tips<!-- /tips --> |
| `baton template` | the handoff sections |
| `baton save @task --title T` | save the handoff (body on stdin) |
| `baton docs list\|show @task [ID]` | the task's docs |
| `baton docs new @task --title T --origin user\|agent` | new doc (body on stdin) |
| `baton docs edit\|delete @task ID` | replace (body on stdin) or remove a doc |
| `baton history @task [N]` | earlier handoffs (1 = newest) |
| `baton done\|restore @task`, `baton rename OLD NEW` | archive, reopen, rename a task |
<!-- tips -->
| `baton tips search\|new\|supersede\|verified` | tips (step 5) |
<!-- /tips -->

`dashboard`, `service`, `auth`, `update`, `integrate`, `config` are for the
user: run them only when asked.

## 1. Pick the task

The `@task` from the arguments; else the task this session worked on; else a
short kebab-case name for the work (`@api-refactor`). If several fit, ask.

## 2. Read the previous handoff

`baton show @task`. Keep only what the next steps still need. Drop what this
session finished and what lives in git, CLAUDE.md, README<!-- tips -->, tips<!-- /tips --> or docs. Steps
ticked `[x]` (the user does that in the dashboard) are done: move what still
matters into State. History keeps only the last few versions, so open
decisions and parked items stay in the handoff.

## 3. Collect what would be lost

The next session sees only the repo, git, the user's own directories and
baton. Everything else this session produced is lost unless saved: review
results, analyses, research findings, test or benchmark output, the reasons
behind a decision, a plan the user gave in chat. List these for step 5.

- `/tmp` and other temporary paths this session wrote to are wiped: every
  text file there worth keeping becomes a doc.
- Save text in full, however long. Over ~8000 characters, split it into
  several docs by section ("Research: update flow (1/3)"), not into a summary.
- A binary file: the handoff names its path as temporary.
- Skip what the repo, git log, CLAUDE.md or README already hold.

## 4. Write and save the handoff

Sections come from `baton template`; Goal, State and Next steps are required,
skip empty ones. Write for a glance:

- About 3000 characters; one fact per bullet, under 200 characters, at most
  one level of nesting.
- State is where things stand now, not the story of the session.
- Next steps are `1. [ ] ...` items in order; Later stays plain bullets.
- `path:line` instead of code; a doc as `[[doc-id]]`<!-- tips -->, a tip by its ID<!-- /tips -->.

Good: `- update: TLS fails on Termux, fix in internal/update/http.go:40 (not started)`
Bad: `- This session we looked into update and found that on Termux, because ...`

```bash
baton save @task --title "Short title" <<'HANDOFF'
## Goal
...
HANDOFF
```

If baton warns about missing sections or the size, fix the body and save again.

## 5. Save docs

A doc is attached to the task and outlives every handoff; `baton show` lists
it. Check `baton docs list @task` first and update a doc instead of adding a
near-duplicate.

```bash
baton docs new @task --title "Review of the update flow" --origin agent < /tmp/review.md
baton docs new @task --title "Migration plan" --origin user <<'DOC'
1. ...
DOC
```

- `--origin user`: the user's text (a plan, rules, an agreement), copied as
  is. Change or delete it only with the user's consent.
- `--origin agent`: text you wrote or collected. Update it with
  `baton docs edit @task ID` (full body on stdin) or delete it when stale.
- A handoff section that stayed the same for two handoffs and is over ~10
  lines also moves into a doc.
<!-- tips -->

## 6. Tips

Most sessions add none. For a non-obvious trap that would help other
sessions (a misleading error, a flag that does not work, an environment
quirk):

1. `baton tips search WORDS` for duplicates. A wrong or outdated tip: write
   the new one, then `baton tips supersede OLD NEW`; a tip this session
   confirmed: `baton tips verified ID`.
2. Otherwise add one fact per tip (`--global` if not specific to this
   project), title under 70 characters, up to 8 keywords. A stable user
   preference not tied to this task is a tip with `origin: user`.

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

`origin`: `session` (found here), `failure` (an error hit), `web` (docs),
`user` (the user said so).
<!-- /tips -->

## Modes

- `@task done`: save a final handoff if anything changed, then `baton done @task`.
- `fork NEW`: save the current task, then `baton save @NEW --from parent`.

## Report

Tell the user the task name, the docs saved or changed, and that `/pickup @task`
(or `baton pickup <project> @task` from any directory) continues it.
