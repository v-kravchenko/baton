package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/dashboard"
	"github.com/v-kravchenko/baton/internal/service"
)

func (app *App) dashboardAddr(a *args) (host string, port int, err error) {
	host, port = app.Cfg.Host, app.Cfg.Port
	if v := a.vals["host"]; v != "" {
		host = v
	}
	if v := a.vals["port"]; v != "" {
		if port, err = strconv.Atoi(v); err != nil || port <= 0 || port > 65535 {
			return "", 0, usageErr("invalid --port %q", v)
		}
	}
	return host, port, nil
}

func dashboardURL(host string, port int) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)) + "/"
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func cmdDashboard(app *App, a *args) error {
	if len(a.pos) > 0 {
		return usageErr("unexpected argument %q", a.pos[0])
	}
	state := app.Dirs.State
	if a.bools["stop"] {
		pid, err := service.Stop(state)
		if err != nil {
			return err
		}
		if pid == 0 {
			app.printf("dashboard is not running\n")
		} else {
			app.printf("stopped dashboard (pid %d)\n", pid)
		}
		return nil
	}
	host, port, err := app.dashboardAddr(a)
	if err != nil {
		return err
	}
	url := dashboardURL(host, port)
	if pid := service.Running(state); pid != 0 {
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"url": url, "pid": pid, "running": true})
		}
		app.printf("dashboard already running (pid %d): %s\n", pid, url)
		if !a.bools["no-open"] && !a.bools["background"] {
			app.openURL(url)
		}
		return nil
	}
	if a.bools["background"] {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		args := []string{"dashboard", "--no-open", "--host", host, "--port", strconv.Itoa(port)}
		pid, err := service.Background(state, exe, args)
		if err != nil {
			return err
		}
		addr := net.JoinHostPort(strings.Trim(dialHost(host), "[]"), strconv.Itoa(port))
		if !waitListening(addr, 5*time.Second) {
			return fmt.Errorf("dashboard did not start; see %s", service.LogFile(state))
		}
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"url": url, "pid": pid, "log": service.LogFile(state)})
		}
		app.printf("dashboard running in the background (pid %d): %s\nlog: %s\nstop: baton dashboard --stop\n", pid, url, service.LogFile(state))
		if !a.bools["no-open"] {
			app.openURL(url)
		}
		return nil
	}
	return app.serveDashboard(host, port, url, a)
}

func dialHost(h string) string {
	switch h {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	}
	return h
}

func waitListening(addr string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			c.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (app *App) openURL(url string) {
	if err := service.OpenBrowser(url); err != nil {
		app.printf("open %s in a browser (%v)\n", url, err)
	}
}

func (app *App) serveDashboard(host string, port int, url string, a *args) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listen: %v (is another dashboard running? baton dashboard --stop)", err)
	}
	if err := service.WritePID(app.Dirs.State); err != nil {
		app.warnf("cannot write pid file: %v", err)
	}
	defer service.RemovePID(app.Dirs.State)

	srv := dashboard.New(app.Cfg, app.Dirs, app.store(), app.tipStore(), buildVersion())
	dirs := app.Dirs
	srv.LoadConfig = func() (config.Config, error) { return config.Load(dirs) }
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: time.Minute, IdleTimeout: 2 * time.Minute}

	if !isLoopbackHost(host) && (!app.Cfg.Auth || !srv.Auth.HasPassword()) {
		app.warnf("listening on %s with auth off: remote clients get 403 until `baton auth password` and `baton auth on`", host)
	}
	if a.bools["json"] {
		_ = app.writeJSON(map[string]any{"url": url, "pid": os.Getpid(), "public_url": app.Cfg.PublicURL})
	} else {
		app.printf("baton dashboard: %s", url)
		if app.Cfg.PublicURL != "" {
			app.printf(" (public: %s)", app.Cfg.PublicURL)
		}
		app.printf("\n")
	}
	if !a.bools["no-open"] {
		app.openURL(url)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}
	return nil
}

func cmdService(app *App, a *args) error {
	if len(a.pos) != 1 {
		return usageErr("expected install|status|restart|uninstall")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	plan, err := service.Action(app.serviceSpec(exe), a.pos[0])
	if err != nil {
		return err
	}
	if a.bools["dry-run"] {
		plan.Print(app.Stdout)
		return nil
	}
	if a.pos[0] == "install" {
		if pid := service.Running(app.Dirs.State); pid != 0 {
			app.printf("stopping background dashboard (pid %d)\n", pid)
			_, _ = service.Stop(app.Dirs.State)
		}
	}
	return plan.Apply(app.Stdout)
}

func cmdAuth(app *App, a *args) error {
	if len(a.pos) != 1 {
		return usageErr("expected status|on|off|password|logout-all")
	}
	auth := &dashboard.Auth{StateDir: app.Dirs.State, Idle: app.Cfg.AuthIdle, Max: app.Cfg.AuthMax, Now: app.now}
	cfgFile := app.Dirs.ConfigFile()
	switch a.pos[0] {
	case "status":
		st := map[string]any{"auth": app.Cfg.Auth, "password_set": auth.HasPassword(), "sessions": auth.SessionCount(),
			"idle": app.Cfg.AuthIdle.String(), "max": app.Cfg.AuthMax.String(), "host": app.Cfg.Host, "public_url": app.Cfg.PublicURL}
		if a.bools["json"] {
			return app.writeJSON(st)
		}
		onoff := "off (only loopback clients have access)"
		if app.Cfg.Auth {
			onoff = "on (remote clients log in with the password)"
		}
		app.printf("auth: %s\npassword: %v\nsessions: %d\nidle timeout: %s, max age: %s\n", onoff, auth.HasPassword(), auth.SessionCount(), app.Cfg.AuthIdle, app.Cfg.AuthMax)
		if app.Cfg.Auth && !auth.HasPassword() {
			app.printf("warning: auth is on but no password is set; run baton auth password\n")
		}
		return nil
	case "on":
		if !auth.HasPassword() {
			return errors.New("set a password first: baton auth password")
		}
		if err := config.SetKey(cfgFile, "dashboard.auth", "on"); err != nil {
			return err
		}
		app.printf("auth on: remote clients must log in (%s)\n", cfgFile)
		if isLoopbackHost(app.Cfg.Host) && app.Cfg.PublicURL == "" {
			app.printf("note: dashboard.host is %s; set dashboard.host = 0.0.0.0 (or a LAN address) or dashboard.public_url for remote access, then baton service restart\n", app.Cfg.Host)
		}
		return nil
	case "off":
		if err := config.SetKey(cfgFile, "dashboard.auth", "off"); err != nil {
			return err
		}
		app.printf("auth off: only loopback clients have access\n")
		return nil
	case "password":
		pw, err := app.readPassword()
		if err != nil {
			return err
		}
		if err := auth.SetPassword(pw); err != nil {
			return err
		}
		app.printf("password set; all sessions logged out\n")
		if !app.Cfg.Auth {
			app.printf("enable remote login: baton auth on\n")
		}
		return nil
	case "logout-all":
		if err := auth.LogoutAll(); err != nil {
			return err
		}
		app.printf("all dashboard sessions logged out\n")
		return nil
	}
	return usageErr("unknown auth subcommand %q", a.pos[0])
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// readPassword reads a new password: twice without echo on a terminal, or
// the first line of stdin otherwise.
func (app *App) readPassword() (string, error) {
	f, ok := app.Stdin.(*os.File)
	if !ok || !isTerminal(f) {
		line, err := bufio.NewReader(app.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	read := func(prompt string) (string, error) {
		fmt.Fprint(app.Stderr, prompt)
		restore := noEcho(f)
		line, err := bufio.NewReader(f).ReadString('\n')
		restore()
		fmt.Fprintln(app.Stderr)
		return strings.TrimRight(line, "\r\n"), err
	}
	p1, err := read("new dashboard password: ")
	if err != nil {
		return "", err
	}
	p2, err := read("repeat: ")
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passwords do not match")
	}
	return p1, nil
}

// noEcho turns terminal echo off via stty where available.
func noEcho(f *os.File) func() {
	if runtime.GOOS == "windows" {
		return func() {}
	}
	stty := func(arg string) error {
		cmd := exec.Command("stty", arg)
		cmd.Stdin = f
		return cmd.Run()
	}
	if stty("-echo") != nil {
		return func() {}
	}
	return func() { _ = stty("echo") }
}
