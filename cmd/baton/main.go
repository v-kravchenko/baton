// Command baton stores agent session handoffs as Markdown per named task.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/update"
)

// App carries process-wide state for a command.
type App struct {
	Dirs   config.Dirs
	Cfg    config.Config
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Cwd    string
}

type command struct {
	run   func(*App, *args) error
	flags map[string]bool
	usage string
}

var commands map[string]command

func init() {
	commands = map[string]command{
		"update":   {cmdUpdate, map[string]bool{"check": false, "force": false}, "baton update [--check] [--force]"},
		"save":     {cmdSave, map[string]bool{"from": true, "title": true}, "baton save [@task] [--from PARENT] [--title T] < body.md"},
		"show":     {cmdShow, nil, "baton show [@task|FILE]"},
		"tasks":    {cmdTasks, nil, "baton tasks"},
		"done":     {cmdDone, nil, "baton done TASK"},
		"restore":  {cmdRestore, nil, "baton restore TASK"},
		"rename":   {cmdRename, nil, "baton rename OLD NEW"},
		"history":  {cmdHistory, nil, "baton history TASK [N]"},
		"stale":    {cmdStale, nil, "baton stale FILE|@task"},
		"template": {cmdTemplate, nil, "baton template"},
		"version":  {cmdVersion, nil, "baton version"},
		"tips": {cmdTips, map[string]bool{"error": false, "all": false, "global": false, "title": true, "when": true,
			"keywords": true, "cites": true, "origin": true, "env": true}, "baton tips search|list|show|new|verified|refuted|supersede|move"},
		"path":   {cmdPath, nil, "baton path set KEY DIR | list | prune"},
		"pickup": {cmdPickup, map[string]bool{"agent": true, "print": false}, "baton pickup [PROJECT] [@task] [--agent X] [--print]"},
		"dashboard": {cmdDashboard, map[string]bool{"port": true, "host": true, "no-open": false, "background": false, "stop": false},
			"baton dashboard [--port N] [--host H] [--no-open] [--background|--stop] [--json]"},
		"service":   {cmdService, map[string]bool{"dry-run": false}, "baton service install|status|restart|uninstall [--dry-run]"},
		"auth":      {cmdAuth, nil, "baton auth status|on|off|password|logout-all"},
		"integrate": {cmdIntegrate, map[string]bool{"uninstall": false, "force": false}, "baton integrate claude|opencode [--uninstall] [--force]"},
	}
}

const usageText = `baton — handoffs between agent sessions

usage:
  baton save [@task] [--from PARENT] [--title T]   body from stdin
  baton show [@task|FILE]
  baton tasks | done TASK | restore TASK | rename OLD NEW | history TASK [N] | stale FILE
  baton pickup [PROJECT] [@task] [--agent X] [--print]
  baton path set KEY DIR | list | prune
  baton tips search [--error] WORDS | show ID | new [--global] | verified ID |
             refuted ID WHY | supersede OLD NEW | move ID global|project | list
  baton dashboard [--port N] [--host H] [--no-open] [--background|--stop]
  baton service install|status|restart|uninstall [--dry-run]
  baton auth status|on|off|password|logout-all
  baton integrate claude|opencode [--uninstall]
  baton template
  baton update [--check]
  baton version

common flags: --dir DIR (project directory, default cwd), --json
`

func main() {
	update.CleanupOld()
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(argv) == 0 || argv[0] == "help" || argv[0] == "--help" || argv[0] == "-h" {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	if argv[0] == "--version" {
		argv = []string{"version"}
	}
	cmd, ok := commands[argv[0]]
	if !ok {
		fmt.Fprintf(stderr, "baton: unknown command %q\n\n%s", argv[0], usageText)
		return 2
	}
	a, err := parseArgs(argv[1:], cmd.flags)
	if err == nil {
		var app *App
		app, err = newApp(a, stdin, stdout, stderr)
		if err == nil {
			err = cmd.run(app, a)
		}
	}
	var ec exitCode
	if errors.As(err, &ec) {
		return int(ec)
	}
	if err != nil {
		fmt.Fprintf(stderr, "baton: %v\n", err)
		var ue usageError
		if errors.As(err, &ue) {
			if cmd.usage != "" {
				fmt.Fprintf(stderr, "usage: %s\n", cmd.usage)
			}
			return 2
		}
		return 1
	}
	return 0
}

func newApp(a *args, stdin io.Reader, stdout, stderr io.Writer) (*App, error) {
	app := &App{Dirs: config.ResolveDirs(), Stdin: stdin, Stdout: stdout, Stderr: stderr}
	cfg, err := config.Load(app.Dirs)
	if err != nil {
		return nil, err
	}
	app.Cfg = cfg
	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "baton: warning: %s\n", w)
	}
	cwd := a.vals["dir"]
	if cwd == "" {
		if cwd, err = os.Getwd(); err != nil {
			return nil, err
		}
	}
	if cwd, err = filepath.Abs(cwd); err != nil {
		return nil, err
	}
	app.Cwd = cwd
	return app, nil
}

func (app *App) printf(format string, a ...any) { fmt.Fprintf(app.Stdout, format, a...) }

func (app *App) warnf(format string, a ...any) {
	fmt.Fprintf(app.Stderr, "baton: warning: "+format+"\n", a...)
}

func (app *App) writeJSON(v any) error {
	enc := json.NewEncoder(app.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
