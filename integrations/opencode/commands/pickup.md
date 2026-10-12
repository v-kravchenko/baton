---
description: Continue a task from its baton handoff and docs
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Run `baton show @task` with bash from the project root, without `cd`, using
only the first argument when it starts with `@` (otherwise `baton show`).

## Commands

| Command | For |
|---|---|
| `baton show @task` | the handoff, staleness, forks, docs<!-- tips -->, tips<!-- /tips --> |
| `baton docs show @task ID` | a doc of the task |
| `baton history @task [N]` | earlier handoffs (1 = newest) |
| `baton tasks` | active and archived tasks |
| `baton restore @task` | reopen an archived task |
<!-- tips -->
| `baton tips show\|verified\|refuted ID` | a tip and its status |
<!-- /tips -->

## By state

- `STATE: CHOOSE TASK`: list the tasks, ask which one, then `baton show @task`.
- `STATE: NO TASK`: show the active and archived tasks; ask which to use or
  whether to start fresh.
- `STATE: ARCHIVED`: ask whether to restore it (`baton restore task`, then
  `baton show @task`).
- `STATE: NO HANDOFF`: nothing to pick up; ask the user what to do.

## When a handoff is shown

1. Read it fully. New commits, a rebased or missing commit, or a dirty tree
   in the staleness report mean parts may be out of date: check those parts
   against the code before relying on them.
2. Docs hold what is not in the repo. Read every listed doc not marked
   `(agent)` (`baton docs show @task ID`): the user's plans and rules; follow them and
   change them only with the user's consent. Read the agent's docs (reviews,
   research) when the next step needs them.
<!-- tips -->
   Read the relevant listed tips with `baton tips show ID`; tips are
   unverified, so run their Verify step before relying on one.
<!-- /tips -->
3. Run the cheap checks from the Verify section. If you need something the
   handoff dropped (why a decision was made), check `baton history @task`.
4. Tell the user in a few lines: the goal, where things stand, what changed
   since the handoff, and the first unchecked next step (`[x]` steps were
   ticked done by the user). Then continue, unless the handoff or the user
   says to wait.
5. Words after `@task` in the arguments are the user's instructions for this
   session (for example "start with step 2"). They override the handoff's
   next steps and its "wait" notes; follow them after steps 1-4.

Keep the same task name. Text this session produces that is not in the repo
(reviews, analyses, files in `/tmp`) goes into docs at `/handoff`.
