package main

import (
	"os/exec"

	"github.com/v-kravchenko/baton/integrations"
	"github.com/v-kravchenko/baton/internal/config"
)

func cmdIntegrate(app *App, a *args) error {
	if len(a.pos) != 1 {
		return usageErr("expected an agent: %v", integrations.Names())
	}
	in, err := integrations.Get(a.pos[0])
	if err != nil {
		return err
	}
	if a.bools["uninstall"] {
		removed, err := in.Uninstall(app.Dirs.Home)
		for _, r := range removed {
			app.printf("removed %s\n", r)
		}
		if err == nil && len(removed) == 0 {
			app.printf("%s integration is not installed\n", in.Name)
		}
		if err != nil {
			return err
		}
		return removeAgent(app, in.Name)
	}
	written, err := in.Install(app.Dirs.Home, a.bools["force"])
	for _, w := range written {
		app.printf("wrote %s\n", w)
	}
	if err != nil {
		return err
	}
	if err := addAgent(app, in.Name); err != nil {
		return err
	}
	if _, lerr := exec.LookPath("baton"); lerr != nil {
		app.warnf("baton is not on PATH; the %s skills call `baton` and will fail until it is", in.Name)
	}
	switch in.Name {
	case "claude":
		app.printf("Claude Code: /handoff, /pickup and /tips are available in new sessions\n")
	case "opencode":
		app.printf("opencode: /handoff, /pickup and /tips are available in new sessions\n")
	}
	return nil
}

// addAgent creates the config if needed and appends agent.<name> with the
// built-in template unless the config already has a line for it.
func addAgent(app *App, name string) error {
	tmpl, ok := config.DefaultAgentTemplate(name)
	if !ok {
		return nil
	}
	file, key := app.Dirs.ConfigFile(), "agent."+name
	if _, err := config.EnsureFile(file); err != nil {
		return err
	}
	if _, has, err := config.FileValue(file, key); err != nil || has {
		return err
	}
	if err := config.SetKey(file, key, tmpl); err != nil {
		return err
	}
	app.printf("added %s to %s\n", key, file)
	return nil
}

// removeAgent drops agent.<name> if it still has the built-in template (an
// edited line is the user's) and agent.default if it names the agent.
func removeAgent(app *App, name string) error {
	tmpl, ok := config.DefaultAgentTemplate(name)
	if !ok {
		return nil
	}
	file, key := app.Dirs.ConfigFile(), "agent."+name
	v, has, err := config.FileValue(file, key)
	if err != nil || !has {
		return err
	}
	if v != tmpl {
		app.printf("kept %s in %s (edited by hand)\n", key, file)
		return nil
	}
	if _, err := config.RemoveKey(file, key); err != nil {
		return err
	}
	app.printf("removed %s from %s\n", key, file)
	if d, _, err := config.FileValue(file, "agent.default"); err == nil && d == name {
		if _, err := config.RemoveKey(file, "agent.default"); err != nil {
			return err
		}
		app.printf("removed agent.default from %s\n", file)
	}
	return nil
}
