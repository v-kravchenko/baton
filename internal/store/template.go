package store

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Sections are the handoff template headings, in order. Required ones must
// be present; the others are skipped when there is nothing to say.
var (
	Sections = []string{"Goal", "State", "Next steps", "Decisions", "Context", "Verify"}
	Required = []string{"Goal", "State", "Next steps"}
)

// Size budget of a handoff body, in characters: save warns above it.
const (
	MaxChars = 3000
	MaxLine  = 200
)

// Template is the handoff body template printed by `baton template`.
const Template = `# <Short title of the task>

## Goal
What the task achieves and why. One or two sentences.

## State
Where the work stands now, not the story of the session:
- Done: what is finished and not yet visible in git (unpushed, unreleased).
- In progress: what is half done, with path:line.
- Blocked: what waits on whom.

## Next steps
1. Ordered, concrete actions. The first one is where the next session starts.
- Later: parked items, one line each.

## Decisions
Optional. Choice and reason, one bullet each, so the next session does not
re-litigate it. Mark open questions as OPEN.

## Context
Optional. Facts and traps the next session cannot cheaply rediscover:
constraints, commands, errors hit and their fix.

## Verify
Optional. Commands that prove the state described above.
`

// SizeWarnings reports a body over the size budget: too many characters or
// lines too long to read at a glance. Fenced code blocks are not counted.
func SizeWarnings(body string) []string {
	var out []string
	chars, long, fence := 0, 0, false
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		n := utf8.RuneCountInString(l)
		chars += n + 1
		if n > MaxLine {
			long++
		}
	}
	if chars > MaxChars {
		out = append(out, fmt.Sprintf("handoff is %d characters, budget %d: drop what git, CLAUDE.md, README or tips already hold, and finished work", chars, MaxChars))
	}
	if long > 0 {
		out = append(out, fmt.Sprintf("%d lines over %d characters: one fact per bullet", long, MaxLine))
	}
	return out
}

// MissingSections returns required sections absent from body (matched as
// "## <name>" headings, case-insensitively).
func MissingSections(body string) []string {
	have := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
			have[strings.ToLower(strings.TrimSpace(h))] = true
		}
	}
	var out []string
	for _, s := range Required {
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
