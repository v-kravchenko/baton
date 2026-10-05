---
description: Continue a task from its baton handoff
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Run `baton show @task` with bash from the project root, without `cd`, using
only the first argument when it starts with `@` (otherwise `baton show`).
Words after `@task` are the user's instructions for this session: follow them
after loading the handoff; they override its next steps.

- `STATE: CHOOSE TASK`: ask the user which task, then run `baton show @task`.
- `STATE: NO TASK`: show the active and archived tasks; ask which to use or
  whether to start fresh.
- `STATE: ARCHIVED`: ask whether to restore it (`baton restore task`, then
  `baton show @task`).
- `STATE: NO HANDOFF`: tell the user there is nothing to pick up.

When a handoff is shown: read it fully. Treat new commits, a rebased or
missing commit, or a dirty tree in the staleness report as signs that parts
may be out of date. Read the linked docs with `baton docs show ID` (follow them; change
them only with the user's consent). Read the listed tips with `baton tips show ID` and run
their Verify step before relying on them. Run the cheap Verify checks. If
the handoff lacks something you need, `baton history task` lists earlier
versions (1 = newest) and `baton history task N` prints one. Then
tell the user the goal, the state, what changed, and the first unchecked next
step (`[x]` steps were ticked done by the user), and continue.
