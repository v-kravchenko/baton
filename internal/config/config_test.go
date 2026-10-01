package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseEntriesCRLFAndComments(t *testing.T) {
	data := []byte("# top\r\nroot = ~/Sync/h\r\nagent.x = x \"/pickup {task}\"   # note\r\nbad line\r\nurl = http://a/#frag\r\n")
	es, warns := ParseEntries(data)
	if len(warns) != 1 {
		t.Fatalf("warns = %v", warns)
	}
	want := []Entry{{"root", "~/Sync/h", 2}, {"agent.x", `x "/pickup {task}"`, 3}, {"url", "http://a/#frag", 5}}
	if len(es) != len(want) {
		t.Fatalf("entries = %+v", es)
	}
	for i := range want {
		if es[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, es[i], want[i])
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BATON_ROOT", "")
	cfg := "root = ~/h\nkeep = 3\ndashboard.auth = on\ndashboard.auth.idle = 1d12h\nagent.default = b\nagent.a = a {task}\nagent.b = b\nfoo = 1\n"
	os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o644)
	c, err := Load(Dirs{Home: "/home/u", Config: dir})
	if err != nil {
		t.Fatal(err)
	}
	if c.Root != filepath.Join("/home/u", "h") || c.Keep != 3 || !c.Auth || c.AuthIdle != 36*time.Hour {
		t.Errorf("config = %+v", c)
	}
	if len(c.Agents) != 2 || c.Agents[0].Name != "a" || c.Agents[1].Name != "b" || c.DefaultAgent != "b" {
		t.Errorf("agents = %+v default %q", c.Agents, c.DefaultAgent)
	}
	if len(c.Warnings) != 1 {
		t.Errorf("warnings = %v", c.Warnings)
	}
	if a, err := c.Agent(""); err != nil || a.Name != "b" {
		t.Errorf("default agent = %+v %v", a, err)
	}
	if _, err := c.Agent("zzz"); err == nil {
		t.Error("unknown agent accepted")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("BATON_ROOT", "/r")
	c, err := Load(Dirs{Home: "/home/u", Config: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if c.Root != "/r" || c.Keep != 10 || c.Port != 8765 || c.DefaultAgent != "claude" || len(c.Agents) != 2 {
		t.Errorf("defaults = %+v", c)
	}
}

func TestParsePathsWindows(t *testing.T) {
	p := ParsePaths([]byte("api=C:\\work\\api\r\n# c\r\nweb = /home/u/web\r\nbroken\r\n"))
	if p["api"] != `C:\work\api` || p["web"] != "/home/u/web" || len(p) != 2 {
		t.Errorf("paths = %v", p)
	}
	if got := string(p.Format()); got != "api=C:\\work\\api\nweb=/home/u/web\n" {
		t.Errorf("format = %q", got)
	}
}

func TestRememberAndSetKey(t *testing.T) {
	d := Dirs{Home: "/h", Config: t.TempDir(), State: t.TempDir()}
	if err := Remember(d, "a", "/x/a"); err != nil {
		t.Fatal(err)
	}
	Remember(d, "a", "/y/a")
	p, _ := LoadPaths(d)
	if p["a"] != "/y/a" {
		t.Errorf("paths = %v", p)
	}
	f := d.ConfigFile()
	os.WriteFile(f, []byte("# c\r\ndashboard.auth = off # x\r\nkeep = 2\r\n"), 0o644)
	SetKey(f, "dashboard.auth", "on")
	SetKey(f, "dashboard.port", "9000")
	got, _ := os.ReadFile(f)
	if string(got) != "# c\ndashboard.auth = on\nkeep = 2\ndashboard.port = 9000\n" {
		t.Errorf("config = %q", got)
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{"7d": 168 * time.Hour, "90m": 90 * time.Minute, "1d1h": 25 * time.Hour} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("%s = %v %v", in, got, err)
		}
	}
	if _, err := ParseDuration("x"); err == nil {
		t.Error("x accepted")
	}
}
