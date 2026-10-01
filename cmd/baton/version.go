package main

import (
	"runtime"
	"runtime/debug"
)

// version is set with -ldflags "-X main.version=..." by release builds.
var version = "dev"

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func cmdVersion(app *App, a *args) error {
	if a.bools["json"] {
		return app.writeJSON(map[string]string{"version": buildVersion(), "os": runtime.GOOS, "arch": runtime.GOARCH})
	}
	app.printf("baton %s %s/%s\n", buildVersion(), runtime.GOOS, runtime.GOARCH)
	return nil
}
