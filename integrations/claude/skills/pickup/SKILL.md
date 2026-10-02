---
name: pickup
description: Continue a task from its baton handoff. Use on /pickup [@task], or when the user asks to resume, continue or pick up earlier work in this project.
argument-hint: "[@task] [prompt]"
arguments: [task]
allowed-tools: Bash(baton *)
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

!`baton show $task`

The output above comes from `baton show`. If it is missing, run
`baton show @task` (or `baton show` without a task) yourself from the project
root, without `cd`.

## By state

- `STATE: CHOOSE TASK`: list the tasks and ask the user which one, then run
  `baton show @task`.
- `STATE: NO TASK`: the task does not exist. Show the active and archived
  tasks and ask which to use, or whether to start fresh.
- `STATE: ARCHIVED`: the task is done. Ask whether to restore it
  (`baton restore task`, then `baton show @task`).
- `STATE: NO HANDOFF`: nothing to pick up; tell the user and ask what to do.

## When a handoff is shown

1. Read it fully. Check the staleness report: new commits, a rebased or
   missing commit, or a dirty tree mean parts of the handoff may be out of
   date. Check those parts against the code before relying on them.
2. If tips are listed, read the relevant ones with `baton tips show ID`.
   Tips are unverified: run their Verify step before relying on them.
3. Run the cheap checks from the Verify section.
   If the handoff lacks something you need (why a decision was made, an item
   it dropped), `baton history task` lists earlier versions (1 = newest) and
   `baton history task N` prints one. Only the last few are kept.
4. Tell the user in a few lines: the goal, where things stand, what changed
   since the handoff, and the first next step. Then continue, unless the
   handoff or the user says to wait.
5. Words after `@task` in the arguments are the user's instructions for this
   session (for example "start with step 2"). They override the handoff's
   next steps and its "wait" notes; follow them after steps 1-4.

Keep using the same task name, and save with `/handoff` before the session
ends.
