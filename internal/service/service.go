// Package service runs the dashboard in the background, installs it as a
// per-user service (systemd --user, launchd, Task Scheduler) and opens URLs.
package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// IsTermux reports whether we run inside Termux on Android.
func IsTermux() bool {
	return strings.Contains(os.Getenv("PREFIX"), "com.termux") || os.Getenv("TERMUX_VERSION") != ""
}

func isWSL() bool { return os.Getenv("WSL_DISTRO_NAME") != "" }

// BrowserCommand returns the command that opens url on this OS.
func BrowserCommand(goos, url string) []string {
	switch {
	case goos == "android" || (goos == "linux" && IsTermux()):
		return []string{"termux-open-url", url}
	case goos == "linux" && isWSL():
		return []string{"wslview", url}
	case goos == "darwin":
		return []string{"open", url}
	case goos == "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	}
	return []string{"xdg-open", url}
}

// OpenBrowser opens url; the error tells the caller to print the URL instead.
func OpenBrowser(url string) error {
	argv := BrowserCommand(runtime.GOOS, url)
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found", argv[0])
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// PIDFile is where a running dashboard records its pid.
func PIDFile(state string) string { return filepath.Join(state, "baton.pid") }

// LogFile receives the output of a background dashboard.
func LogFile(state string) string { return filepath.Join(state, "baton.log") }

// WritePID records the current process.
func WritePID(state string) error {
	if err := os.MkdirAll(state, 0o755); err != nil {
		return err
	}
	return os.WriteFile(PIDFile(state), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// RemovePID removes the pid file if it still names this process.
func RemovePID(state string) {
	if pid, _ := ReadPID(state); pid == os.Getpid() {
		os.Remove(PIDFile(state))
	}
}

// ReadPID returns the recorded pid (0 if none).
func ReadPID(state string) (int, error) {
	data, err := os.ReadFile(PIDFile(state))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// Running returns the pid of a live recorded dashboard (0 if none).
func Running(state string) int {
	pid, err := ReadPID(state)
	if err != nil || pid <= 0 || !alive(pid) {
		return 0
	}
	return pid
}

// Background starts `exe args...` detached, with output to the log file.
func Background(state, exe string, args []string) (int, error) {
	if err := os.MkdirAll(state, 0o755); err != nil {
		return 0, err
	}
	logf, err := os.OpenFile(LogFile(state), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer logf.Close()
	fmt.Fprintf(logf, "--- %s starting %s %s\n", time.Now().Format(time.RFC3339), exe, strings.Join(args, " "))
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Stdin = nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()
	return pid, nil
}

// Stop terminates the recorded dashboard.
func Stop(state string) (int, error) {
	pid := Running(state)
	if pid == 0 {
		os.Remove(PIDFile(state))
		return 0, nil
	}
	if err := terminate(pid); err != nil {
		return pid, err
	}
	for i := 0; i < 50 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if alive(pid) {
		return pid, fmt.Errorf("process %d did not exit", pid)
	}
	os.Remove(PIDFile(state))
	return pid, nil
}

// Plan is what a service action writes and runs.
type Plan struct {
	Files    map[string]string // path → content
	Remove   []string
	Commands [][]string
	// Ignore errors of these commands (e.g. stopping a service that is not running).
	Tolerant map[int]bool
	Note     string
}

// Apply writes files and runs commands, echoing them to out.
func (p Plan) Apply(out io.Writer) error {
	for path, content := range p.Files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s\n", path)
	}
	for i, c := range p.Commands {
		fmt.Fprintf(out, "$ %s\n", strings.Join(c, " "))
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Run(); err != nil && !p.Tolerant[i] {
			return fmt.Errorf("%s: %v", c[0], err)
		}
	}
	for _, f := range p.Remove {
		if err := os.Remove(f); err == nil {
			fmt.Fprintf(out, "removed %s\n", f)
		}
	}
	if p.Note != "" {
		fmt.Fprintln(out, p.Note)
	}
	return nil
}

// Print shows the plan without running it (--dry-run).
func (p Plan) Print(out io.Writer) {
	for path, content := range p.Files {
		fmt.Fprintf(out, "would write %s:\n%s\n", path, content)
	}
	for _, c := range p.Commands {
		fmt.Fprintf(out, "would run: %s\n", strings.Join(c, " "))
	}
	for _, f := range p.Remove {
		fmt.Fprintf(out, "would remove %s\n", f)
	}
}

// Names used for the per-user service.
const (
	unitName  = "baton.service"
	plistName = "com.baton.dashboard"
	taskName  = "baton-dashboard"
)

// Spec describes the service to install.
type Spec struct {
	GOOS string
	Home string
	Exe  string
	Env  []string // KEY=VALUE passed to the service (BATON_* overrides)
	UID  int
}

func (s Spec) unitPath() string {
	return filepath.Join(s.Home, ".config", "systemd", "user", unitName)
}

func (s Spec) plistPath() string {
	return filepath.Join(s.Home, "Library", "LaunchAgents", plistName+".plist")
}

func (s Spec) launchTarget() string { return "gui/" + strconv.Itoa(s.UID) }

var errTermux = errors.New("no service manager on Termux; use: baton dashboard --background")

// Action builds the plan for install|uninstall|restart|status.
func Action(s Spec, action string) (Plan, error) {
	if s.GOOS == "android" || (s.GOOS == "linux" && IsTermux()) {
		return Plan{}, errTermux
	}
	switch s.GOOS {
	case "linux":
		return systemdPlan(s, action)
	case "darwin":
		return launchdPlan(s, action)
	case "windows":
		return schtasksPlan(s, action)
	}
	return Plan{}, fmt.Errorf("service is not supported on %s; use: baton dashboard --background", s.GOOS)
}

func systemdPlan(s Spec, action string) (Plan, error) {
	sc := func(args ...string) []string { return append([]string{"systemctl", "--user"}, args...) }
	switch action {
	case "install":
		var env strings.Builder
		for _, e := range s.Env {
			env.WriteString("Environment=" + systemdQuote(e) + "\n")
		}
		unit := fmt.Sprintf(`[Unit]
Description=baton dashboard
After=network.target

[Service]
ExecStart=%s dashboard --no-open
%sRestart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, systemdQuote(s.Exe), env.String())
		return Plan{
			Files:    map[string]string{s.unitPath(): unit},
			Commands: [][]string{sc("daemon-reload"), sc("enable", "--now", unitName), sc("restart", unitName)},
			Note:     "to keep it running after logout: loginctl enable-linger $USER",
		}, nil
	case "uninstall":
		return Plan{
			Commands: [][]string{sc("disable", "--now", unitName), sc("daemon-reload")},
			Tolerant: map[int]bool{0: true, 1: true},
			Remove:   []string{s.unitPath()},
		}, nil
	case "restart":
		return Plan{Commands: [][]string{sc("restart", unitName)}}, nil
	case "status":
		return Plan{Commands: [][]string{sc("status", "--no-pager", unitName)}, Tolerant: map[int]bool{0: true}}, nil
	}
	return Plan{}, fmt.Errorf("unknown service action %q", action)
}

func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"\\") {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func launchdPlan(s Spec, action string) (Plan, error) {
	target := s.launchTarget()
	switch action {
	case "install":
		var env strings.Builder
		if len(s.Env) > 0 {
			env.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
			for _, e := range s.Env {
				k, v, _ := strings.Cut(e, "=")
				fmt.Fprintf(&env, "    <key>%s</key><string>%s</string>\n", xmlEscape(k), xmlEscape(v))
			}
			env.WriteString("  </dict>\n")
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>dashboard</string><string>--no-open</string></array>
%s  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
</dict>
</plist>
`, plistName, xmlEscape(s.Exe), env.String())
		return Plan{
			Files: map[string]string{s.plistPath(): plist},
			Commands: [][]string{
				{"launchctl", "bootout", target + "/" + plistName},
				{"launchctl", "bootstrap", target, s.plistPath()},
			},
			Tolerant: map[int]bool{0: true},
		}, nil
	case "uninstall":
		return Plan{
			Commands: [][]string{{"launchctl", "bootout", target + "/" + plistName}},
			Tolerant: map[int]bool{0: true},
			Remove:   []string{s.plistPath()},
		}, nil
	case "restart":
		return Plan{Commands: [][]string{{"launchctl", "kickstart", "-k", target + "/" + plistName}}}, nil
	case "status":
		return Plan{Commands: [][]string{{"launchctl", "print", target + "/" + plistName}}, Tolerant: map[int]bool{0: true}}, nil
	}
	return Plan{}, fmt.Errorf("unknown service action %q", action)
}

func schtasksPlan(s Spec, action string) (Plan, error) {
	switch action {
	case "install":
		tr := `"` + s.Exe + `" dashboard --no-open`
		p := Plan{
			Commands: [][]string{
				{"schtasks", "/create", "/tn", taskName, "/tr", tr, "/sc", "onlogon", "/rl", "limited", "/f"},
				{"schtasks", "/run", "/tn", taskName},
			},
		}
		if len(s.Env) > 0 {
			p.Note = "note: Task Scheduler does not pass BATON_* overrides; set them as user environment variables (setx)"
		}
		return p, nil
	case "uninstall":
		return Plan{
			Commands: [][]string{{"schtasks", "/end", "/tn", taskName}, {"schtasks", "/delete", "/tn", taskName, "/f"}},
			Tolerant: map[int]bool{0: true},
		}, nil
	case "restart":
		return Plan{
			Commands: [][]string{{"schtasks", "/end", "/tn", taskName}, {"schtasks", "/run", "/tn", taskName}},
			Tolerant: map[int]bool{0: true},
		}, nil
	case "status":
		return Plan{Commands: [][]string{{"schtasks", "/query", "/tn", taskName, "/v", "/fo", "list"}}, Tolerant: map[int]bool{0: true}}, nil
	}
	return Plan{}, fmt.Errorf("unknown service action %q", action)
}

// Installed reports whether the per-user service is installed.
func Installed(s Spec) bool {
	switch s.GOOS {
	case "linux":
		_, err := os.Stat(s.unitPath())
		return err == nil
	case "darwin":
		_, err := os.Stat(s.plistPath())
		return err == nil
	case "windows":
		return exec.Command("schtasks", "/query", "/tn", taskName).Run() == nil
	}
	return false
}
