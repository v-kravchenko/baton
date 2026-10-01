package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/v-kravchenko/baton/internal/agent"
	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/store"
)

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func cmdPath(app *App, a *args) error {
	if len(a.pos) == 0 {
		return usageErr("expected set|list|prune")
	}
	switch a.pos[0] {
	case "set":
		if len(a.pos) != 3 {
			return usageErr("expected set KEY DIR")
		}
		key, err := store.TaskName(a.pos[1]) // same charset as task names
		if err != nil || key == store.Global {
			return fmt.Errorf("invalid project key %q", a.pos[1])
		}
		dir, err := filepath.Abs(a.pos[2])
		if err != nil {
			return err
		}
		if !isDir(dir) {
			return fmt.Errorf("not a directory: %s", dir)
		}
		if err := config.UpdatePaths(app.Dirs, func(p config.Paths) bool { p[key] = dir; return true }); err != nil {
			return err
		}
		if k, err := store.ProjectKey(dir); err == nil && k != key {
			app.warnf("directory name gives key %q, not %q; save/show there will record it as %q", k, key, k)
		}
		app.printf("%s=%s\n", key, dir)
		return nil
	case "list":
		p, err := config.LoadPaths(app.Dirs)
		if err != nil {
			return err
		}
		if a.bools["json"] {
			return app.writeJSON(p)
		}
		for _, k := range p.Keys() {
			mark := ""
			if !isDir(p[k]) {
				mark = "   (missing)"
			}
			app.printf("%s=%s%s\n", k, p[k], mark)
		}
		return nil
	case "prune":
		var removed []string
		err := config.UpdatePaths(app.Dirs, func(p config.Paths) bool {
			for _, k := range p.Keys() {
				if !isDir(p[k]) {
					removed = append(removed, k+"="+p[k])
					delete(p, k)
				}
			}
			return len(removed) > 0
		})
		if err != nil {
			return err
		}
		if a.bools["json"] {
			if removed == nil {
				removed = []string{}
			}
			return app.writeJSON(map[string]any{"removed": removed})
		}
		for _, r := range removed {
			app.printf("removed %s\n", r)
		}
		if len(removed) == 0 {
			app.printf("nothing to prune\n")
		}
		return nil
	}
	return usageErr("unknown path subcommand %q", a.pos[0])
}

// pickupTarget resolves the project directory for pickup.
func pickupTarget(app *App, project string) (key, dir string, err error) {
	cwdKey, cwdErr := store.ProjectKey(app.Cwd)
	paths, err := config.LoadPaths(app.Dirs)
	if err != nil {
		return "", "", err
	}
	if project == "" {
		if cwdErr != nil {
			return "", "", cwdErr
		}
		if !app.store().Exists(cwdKey) {
			return "", "", fmt.Errorf("no handoffs for project %q (from the current directory name); cd into the project directory or pass PROJECT", cwdKey)
		}
		app.remember(cwdKey)
		return cwdKey, app.Cwd, nil
	}
	key = project
	if d, ok := paths[key]; ok && isDir(d) {
		return key, d, nil
	}
	if cwdErr == nil && cwdKey == key {
		app.remember(key)
		return key, app.Cwd, nil
	}
	if d, ok := paths[key]; ok {
		return "", "", fmt.Errorf("directory of project %q is missing on this machine: %s; cd into the project and rerun, or: baton path set %s DIR", key, d, key)
	}
	return "", "", fmt.Errorf("project %q has no directory on this machine; cd into it and rerun, or: baton path set %s DIR", key, key)
}

func cmdPickup(app *App, a *args) error {
	task, rest := splitTask(a.pos)
	if len(rest) > 2 {
		return usageErr("too many arguments; quote the prompt")
	}
	project, prompt := "", ""
	if len(rest) >= 1 {
		project = strings.ToLower(rest[0])
		if _, err := store.TaskName(project); err != nil { // same charset as task names
			return usageErr("invalid project %q; a prompt goes after PROJECT and @task", rest[0])
		}
	}
	if len(rest) == 2 {
		prompt = strings.TrimSpace(rest[1])
		if task == "" {
			return usageErr("PROMPT needs @task")
		}
	}
	if task != "" {
		var err error
		if task, err = store.TaskName(task); err != nil {
			return err
		}
	}
	ag, err := app.Cfg.Agent(a.vals["agent"])
	if err != nil {
		return err
	}
	key, dir, err := pickupTarget(app, project)
	if err != nil {
		return err
	}
	v := agent.Vars{Prompt: prompt, Project: key, Dir: dir, Root: app.Cfg.Root}
	if task != "" {
		v.Task = "@" + task
	}
	argv, err := agent.Build(ag.Template, v)
	if err != nil {
		return fmt.Errorf("agent.%s: %v", ag.Name, err)
	}
	if a.bools["print"] {
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"agent": ag.Name, "project": key, "dir": dir, "argv": argv})
		}
		app.printf("cd %s && %s\n", agent.QuoteArg(dir), agent.Quote(argv))
		return nil
	}
	path, err := agent.Resolve(argv)
	if err != nil {
		return err
	}
	code, err := agent.Run(path, argv, dir)
	if err != nil {
		return fmt.Errorf("start %s: %v", argv[0], err)
	}
	if code != 0 {
		return exitCode(code)
	}
	return nil
}

// exitCode propagates a child's exit status without an error message.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
