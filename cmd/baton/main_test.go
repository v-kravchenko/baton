package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"baton": func() int { return run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr) },
	}))
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "script"),
		Setup: func(env *testscript.Env) error {
			work := env.WorkDir
			env.Setenv("HOME", filepath.Join(work, "home"))
			env.Setenv("USERPROFILE", filepath.Join(work, "home"))
			env.Setenv("BATON_ROOT", filepath.Join(work, "root"))
			env.Setenv("BATON_STATE", filepath.Join(work, "state"))
			env.Setenv("BATON_CONFIG_DIR", filepath.Join(work, "cfg"))
			env.Setenv("BATON_NOW", "2026-10-01T10:15:00+03:00")
			env.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			env.Setenv("GIT_AUTHOR_NAME", "t")
			env.Setenv("GIT_AUTHOR_EMAIL", "t@t")
			env.Setenv("GIT_COMMITTER_NAME", "t")
			env.Setenv("GIT_COMMITTER_EMAIL", "t@t")
			return nil
		},
	})
}
