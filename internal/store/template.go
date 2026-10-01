package store

import "strings"

// Sections are the handoff template headings, in order.
var Sections = []string{"Goal", "State", "Decisions", "Key context", "Gotchas", "User preferences", "Next steps", "Verify"}

// Template is the handoff body template printed by `baton template`.
const Template = `# <Short title of the task>

## Goal
What the task is trying to achieve and why. One or two sentences.

## State
Where the work stands: done, in progress, not started. Name files and
functions (path:line) instead of pasting code.

## Decisions
Choices made and their reasons, including rejected alternatives, so the next
session does not re-litigate them. Mark open decisions as OPEN.

## Key context
Facts the next session needs and cannot cheaply rediscover: constraints,
commands, links, data shapes.

## Gotchas
Traps already hit: errors, dead ends, misleading docs, and how they were
resolved.

## User preferences
How the user wants the work done (style, scope, tools, things to avoid).

## Next steps
Ordered, concrete actions to continue with.

## Verify
Commands or checks that prove the current state (tests, builds, URLs).
`

// MissingSections returns template sections absent from body (matched as
// "## <name>" headings, case-insensitively).
func MissingSections(body string) []string {
	have := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
			have[strings.ToLower(strings.TrimSpace(h))] = true
		}
	}
	var out []string
	for _, s := range Sections {
		if !have[strings.ToLower(s)] {
			out = append(out, s)
		}
	}
	return out
}

// Section returns the text under "## name" up to the next "## " heading.
func Section(body, name string) string {
	var b strings.Builder
	in := false
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
			in = strings.EqualFold(strings.TrimSpace(h), name)
			continue
		}
		if in {
			b.WriteString(l + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}
