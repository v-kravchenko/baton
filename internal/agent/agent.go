// Package agent turns agent command templates into argv and launches them.
package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// Vars are the placeholder values for a template.
type Vars struct {
	Task    string // "@name" or ""
	Project string
	Dir     string
	Root    string
}

// Split breaks a template into arguments on whitespace, honoring "..." and
// '...'. Backslash is an ordinary character so Windows paths work as is.
func Split(tmpl string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inArg := false
	var q rune
	for _, r := range tmpl {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			q = r
			inArg = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if q != 0 {
		return nil, fmt.Errorf("unterminated quote in agent template %q", tmpl)
	}
	if inArg {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty agent template")
	}
	return out, nil
}

// Expand substitutes placeholders in every argument separately. Trailing
// spaces are trimmed; arguments that become empty are dropped.
func Expand(args []string, v Vars) []string {
	r := strings.NewReplacer("{task}", v.Task, "{project}", v.Project, "{dir}", v.Dir, "{root}", v.Root)
	out := make([]string, 0, len(args))
	for i, a := range args {
		e := strings.TrimRight(r.Replace(a), " \t")
		if e == "" && a != "" && i > 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Build parses and expands a template.
func Build(tmpl string, v Vars) ([]string, error) {
	args, err := Split(tmpl)
	if err != nil {
		return nil, err
	}
	return Expand(args, v), nil
}

// Resolve finds the program on PATH (PATHEXT on Windows).
func Resolve(argv []string) (string, error) {
	p, err := exec.LookPath(argv[0])
	if err != nil {
		return "", fmt.Errorf("agent program %q not found on PATH", argv[0])
	}
	return p, nil
}

// Quote renders argv as a POSIX shell command line for --print.
func Quote(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = QuoteArg(a)
	}
	return strings.Join(parts, " ")
}

// QuoteArg quotes one argument for a POSIX shell when needed.
func QuoteArg(a string) string {
	if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
		return a
	}
	return "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
}
