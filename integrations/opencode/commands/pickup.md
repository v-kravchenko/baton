---
description: Continue a task from its baton handoff
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Run `baton show $ARGUMENTS` with bash from the project root, without `cd`.

- `STATE: CHOOSE TASK`: ask the user which task, then run `baton show @task`.
- `STATE: NO TASK`: show the active and archived tasks; ask which to use or
  whether to start fresh.
- `STATE: ARCHIVED`: ask whether to restore it (`baton restore task`, then
  `baton show @task`).
- `STATE: NO HANDOFF`: tell the user there is nothing to pick up.

When a handoff is shown: read it fully. Treat new commits, a rebased or
missing commit, or a dirty tree in the staleness report as signs that parts
may be out of date. Read the listed tips with `baton tips show ID` and run
their Verify step before relying on them. Run the cheap Verify checks. Then
tell the user the goal, the state, what changed, and the first next step, and
continue.
