---
description: Search baton tips before debugging, or update a tip's status
---
<!-- managed by baton integrate: edits are overwritten by baton update -->

Arguments: $ARGUMENTS

Run with bash from the project root, without `cd`:

- error output: `baton tips search --error "<key error lines>"`;
- keywords: `baton tips search WORDS`;
- full text: `baton tips show ID`.

Tips are unverified: run a tip's Verify step before acting on it. Then mark it
with `baton tips verified ID` or `baton tips refuted ID WHY`, or save a better
tip and run `baton tips supersede OLD NEW`. If nothing matches, say so and
continue.
