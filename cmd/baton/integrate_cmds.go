package main

import (
	"os/exec"

	"github.com/v-kravchenko/baton/integrations"
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
		return err
	}
	written, err := in.Install(app.Dirs.Home, a.bools["force"])
	for _, w := range written {
		app.printf("wrote %s\n", w)
	}
	if err != nil {
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
