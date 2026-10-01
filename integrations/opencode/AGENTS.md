## baton (handoffs and tips)

- Before debugging an error or a failing command, run `baton tips search --error "<key error lines>"`. Also run `baton tips search WORDS` before changing an area you have not touched this session and when choosing between approaches.
- Tips are unverified hints: run a tip's Verify step before relying on it, then mark it with `baton tips verified ID` or `baton tips refuted ID WHY`.
- Run `baton` from the project root without `cd`. Never write files under the baton root directly; use `/handoff`, `/pickup` and `/tips`.
