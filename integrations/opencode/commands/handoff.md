---
description: Save this session as a baton handoff for a named task
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

All state lives behind the `baton` CLI. Run it with bash from the project root
as is, without `cd`. Never write files under the baton root yourself: pass
bodies to `baton save` and `baton tips new` on stdin.

1. Run `baton tasks`. Pick the task: the `@task` from the arguments; else the
   task this session worked on; else a short kebab-case name for the work. If
   several fit, ask the user.
2. Run `baton show @task`. Keep only what the next steps still need; drop
   what this session finished or what git, CLAUDE.md, README or tips hold.
   History keeps only the last few versions, so open decisions and parked
   items stay in the handoff or move to a tip.
3. Run `baton template`. Goal, State and Next steps are required; skip the
   others when empty. Write for a glance: about 3000 characters, one fact per
   bullet under 200 characters, State is where things stand now (not the
   story of the session), `path:line` instead of code. Do not repeat git log,
   CLAUDE.md, README or tips (name the tip ID). A stable user preference
   becomes a tip with `origin: user`. Save with a quoted heredoc:

   ```bash
   baton save @task --title "Short title" <<'HANDOFF'
   ## Goal
   ...
   HANDOFF
   ```

   If baton warns about missing sections or the size budget, fix and save
   again.
4. Tips: for each non-obvious trap worth keeping, run `baton tips search WORDS`
   for duplicates, then either `baton tips supersede OLD NEW` or
   `baton tips verified ID`, or add one (`--global` if not project-specific;
   one fact, title under 70 characters, up to 8 keywords; `origin` is
   `session`, `failure`, `web` or `user`):

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
   Verify: a quick check that proves it still holds.
   TIP
   ```

Modes: `@task done` saves a final handoff if needed, then `baton done task`.
`fork NEW` saves the current task, then `baton save @NEW --from parent`.

Finish by telling the user the task name and that `/pickup @task` continues it.
