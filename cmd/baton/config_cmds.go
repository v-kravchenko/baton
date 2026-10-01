package main

import "github.com/v-kravchenko/baton/internal/config"

func cmdConfig(app *App, a *args) error {
	if len(a.pos) != 1 {
		return usageErr("expected init or path")
	}
	file := app.Dirs.ConfigFile()
	switch a.pos[0] {
	case "path":
		app.printf("%s\n", file)
	case "init":
		created, err := config.EnsureFile(file)
		if err != nil {
			return err
		}
		if created {
			app.printf("wrote %s\n", file)
		} else {
			app.printf("%s exists; left as is\n", file)
		}
	default:
		return usageErr("unknown config command %q", a.pos[0])
	}
	return nil
}
