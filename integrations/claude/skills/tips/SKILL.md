---
name: tips
description: Search baton tips (unverified gotchas, dead ends and fixes saved by earlier sessions) and update their status. Use BEFORE debugging an error or a failing command (pass the error text), before changing an area you have not touched this session, when choosing between approaches, and for how-to questions about the project. Also to mark a tip verified or refuted.
argument-hint: "<error text or keywords> | show ID | verified ID | refuted ID WHY"
allowed-tools: Bash(baton *)
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Run from the project root, without `cd`:

- error output: `baton tips search --error "<the key error lines>"`;
- keywords: `baton tips search WORDS`;
- full text: `baton tips show ID`.

Tips are hints, not facts. Before acting on one, run its Verify step. Then:

- it held: `baton tips verified ID`;
- it was wrong or no longer applies: `baton tips refuted ID WHY`;
- you found a better answer: save a new tip (see /handoff) and run
  `baton tips supersede OLD NEW`.

`baton tips list` shows the tips of this project and the global ones;
`baton tips move ID global|project` changes a tip's level.

If nothing matches, say so briefly and continue with normal debugging.
