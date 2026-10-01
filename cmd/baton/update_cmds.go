package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/v-kravchenko/baton/integrations"
	"github.com/v-kravchenko/baton/internal/service"
	"github.com/v-kravchenko/baton/internal/update"
)

func (app *App) serviceSpec(exe string) service.Spec {
	spec := service.Spec{GOOS: runtime.GOOS, Home: app.Dirs.Home, Exe: exe, UID: os.Getuid()}
	for _, k := range []string{"BATON_CONFIG_DIR", "BATON_STATE", "BATON_ROOT"} {
		if v := os.Getenv(k); v != "" {
			spec.Env = append(spec.Env, k+"="+v)
		}
	}
	return spec
}

func cmdUpdate(app *App, a *args) error {
	if len(a.pos) > 0 {
		return usageErr("unexpected argument %q", a.pos[0])
	}
	c := update.Client()
	rel, err := update.Latest(c)
	if err != nil {
		return fmt.Errorf("check for updates: %v", err)
	}
	cur := buildVersion()
	same := update.Same(cur, rel.Tag)
	if a.bools["check"] || same {
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"current": cur, "latest": rel.Tag, "update": !same})
		}
		if same {
			app.printf("baton %s is up to date\n", cur)
		} else {
			app.printf("update available: %s → %s (run baton update)\n", cur, rel.Tag)
		}
		return nil
	}
	if cur == "dev" && !a.bools["force"] {
		return fmt.Errorf("this is a development build; pass --force to replace it with %s", rel.Tag)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	bin, err := update.Download(c, rel)
	if err != nil {
		return err
	}
	if err := update.Replace(exe, bin); err != nil {
		return err
	}
	app.printf("updated %s: %s → %s\n", exe, cur, rel.Tag)

	// The new binary carries the new skill files: let it rewrite them.
	for _, in := range integrations.Registry {
		if !in.Installed(app.Dirs.Home) {
			continue
		}
		cmd := exec.Command(exe, "integrate", in.Name)
		cmd.Stdout, cmd.Stderr = app.Stdout, app.Stderr
		if err := cmd.Run(); err != nil {
			app.warnf("refresh %s integration: %v", in.Name, err)
		}
	}
	return app.restartDashboard(exe)
}

// restartDashboard restarts a running service or background dashboard so it
// runs the new binary.
func (app *App) restartDashboard(exe string) error {
	spec := app.serviceSpec(exe)
	if runtime.GOOS != "android" && !service.IsTermux() && service.Installed(spec) {
		plan, err := service.Action(spec, "restart")
		if err != nil {
			return err
		}
		return plan.Apply(app.Stdout)
	}
	if pid := service.Running(app.Dirs.State); pid != 0 {
		if _, err := service.Stop(app.Dirs.State); err != nil {
			return err
		}
		args := []string{"dashboard", "--no-open", "--host", app.Cfg.Host, "--port", strconv.Itoa(app.Cfg.Port)}
		npid, _, err := service.Background(app.Dirs.State, exe, args)
		if err != nil {
			return err
		}
		app.printf("restarted background dashboard (pid %d)\n", npid)
	}
	return nil
}
