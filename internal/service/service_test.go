package service

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPlans(t *testing.T) {
	t.Setenv("PREFIX", "")
	t.Setenv("TERMUX_VERSION", "")
	s := Spec{Home: "/home/u", Exe: "/opt/my tools/baton", Env: []string{"BATON_ROOT=/r"}, UID: 501}
	s.GOOS = "linux"
	p, err := Action(s, "install")
	if err != nil {
		t.Fatal(err)
	}
	unit := p.Files[filepath.Join("/home/u", ".config", "systemd", "user", "baton.service")]
	if !strings.Contains(unit, `ExecStart="/opt/my tools/baton" dashboard --no-open`) || !strings.Contains(unit, "Environment=BATON_ROOT=/r") {
		t.Errorf("unit = %s", unit)
	}
	q := s
	q.Exe, q.Env = "/opt/100%$x/baton", []string{"BATON_ROOT=/r/50%$y"}
	p, _ = Action(q, "install")
	unit = p.Files[filepath.Join("/home/u", ".config", "systemd", "user", "baton.service")]
	if !strings.Contains(unit, "ExecStart=/opt/100%%$$x/baton ") || !strings.Contains(unit, "Environment=BATON_ROOT=/r/50%%$y\n") {
		t.Errorf("escaped unit = %s", unit)
	}
	s.GOOS = "darwin"
	p, _ = Action(s, "install")
	if c := p.Commands[1]; strings.Join(c, " ") != "launchctl bootstrap gui/501 "+filepath.Join("/home/u", "Library", "LaunchAgents", "com.baton.dashboard.plist") {
		t.Errorf("launchd = %v", c)
	}
	s.GOOS = "windows"
	p, _ = Action(s, "install")
	if c := p.Commands[0]; c[0] != "schtasks" || c[5] != `"/opt/my tools/baton" dashboard --no-open` {
		t.Errorf("schtasks = %v", c)
	}
	t.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	s.GOOS = "linux"
	if _, err := Action(s, "install"); err == nil {
		t.Error("termux service accepted")
	}
	if c := BrowserCommand("linux", "http://x"); c[0] != "termux-open-url" {
		t.Errorf("termux browser = %v", c)
	}
	if c := BrowserCommand("windows", "http://x"); c[1] != "url.dll,FileProtocolHandler" {
		t.Errorf("windows browser = %v", c)
	}
}
