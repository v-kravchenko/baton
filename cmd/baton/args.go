package main

import (
	"fmt"
	"strings"
)

// args holds parsed command-line arguments. Flags may appear anywhere;
// `--` ends flag parsing.
type args struct {
	pos   []string
	vals  map[string]string
	bools map[string]bool
}

// commonFlags are accepted by every command. true = takes a value.
var commonFlags = map[string]bool{"dir": true, "json": false}

func parseArgs(in []string, spec map[string]bool) (*args, error) {
	a := &args{vals: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(in); i++ {
		s := in[i]
		if s == "--" {
			a.pos = append(a.pos, in[i+1:]...)
			break
		}
		if !strings.HasPrefix(s, "--") || s == "--" {
			a.pos = append(a.pos, s)
			continue
		}
		name, val, hasVal := strings.Cut(s[2:], "=")
		takes, ok := spec[name]
		if !ok {
			takes, ok = commonFlags[name]
		}
		if !ok {
			return nil, usageErr("unknown flag --%s", name)
		}
		if !takes {
			if hasVal {
				return nil, usageErr("flag --%s takes no value", name)
			}
			a.bools[name] = true
			continue
		}
		if !hasVal {
			if i+1 >= len(in) {
				return nil, usageErr("flag --%s needs a value", name)
			}
			i++
			val = in[i]
		}
		a.vals[name] = val
	}
	return a, nil
}

// splitTask separates a leading `@task` positional from the rest.
func splitTask(pos []string) (task string, rest []string) {
	for i, p := range pos {
		if strings.HasPrefix(p, "@") {
			return p[1:], append(append([]string{}, pos[:i]...), pos[i+1:]...)
		}
	}
	return "", pos
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usageErr(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }
