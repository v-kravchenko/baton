package integrations

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedFilesExistAndAreMarked(t *testing.T) {
	for _, in := range Registry {
		for _, f := range in.Files {
			data, err := Read(f.Src, true)
			if err != nil {
				t.Fatalf("%s: %v", f.Src, err)
			}
			if !strings.Contains(string(data), Marker) {
				t.Errorf("%s lacks the marker", f.Src)
			}
			if strings.Contains(string(data), "CLAUDE_SKILL_DIR") {
				t.Errorf("%s references skill-dir scripts", f.Src)
			}
		}
		for _, f := range append([]File{in.Block}, in.Files...) {
			on, err := Read(f.Src, true)
			if err != nil {
				t.Fatalf("%s: %v", f.Src, err)
			}
			if strings.Contains(string(on), tipsBegin) || strings.Contains(string(on), tipsEnd) {
				t.Errorf("%s has unbalanced tips markers", f.Src)
			}
			if f.Tips {
				continue
			}
			off, _ := Read(f.Src, false)
			if strings.Contains(strings.ToLower(string(off)), "tip") {
				t.Errorf("%s mentions tips with tips off", f.Src)
			}
		}
	}
	// Every embedded file is registered.
	fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		found := false
		for _, in := range Registry {
			found = found || in.Block.Src == p
			for _, f := range in.Files {
				found = found || f.Src == p
			}
		}
		if !found {
			t.Errorf("embedded %s is not in Registry", p)
		}
		return nil
	})
}

func TestCutTips(t *testing.T) {
	text := "a\n<!-- tips -->\nb\n<!-- /tips -->\nc<!-- tips --> d<!-- /tips -->.\n"
	if got := cutTips(text, true); got != "a\nb\nc d.\n" {
		t.Errorf("on: %q", got)
	}
	if got := cutTips(text, false); got != "a\nc.\n" {
		t.Errorf("off: %q", got)
	}
}

func TestInstallUninstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	in, _ := Get("claude")
	base := filepath.Join(home, ".claude")
	os.MkdirAll(filepath.Join(base, "skills", "tips"), 0o755)
	os.WriteFile(filepath.Join(base, "CLAUDE.md"), []byte("# Mine\r\nkeep me\r\n"), 0o644)
	foreign := filepath.Join(base, "skills", "tips", "SKILL.md")
	os.WriteFile(foreign, []byte("someone else's skill"), 0o644)

	written, _, err := in.Install(home, false, true)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("foreign file overwritten: %v", err)
	}
	if len(written) != 3 {
		t.Errorf("written = %v", written)
	}
	if data, _ := os.ReadFile(foreign); string(data) != "someone else's skill" {
		t.Error("foreign file changed")
	}
	if !in.Installed(home) {
		t.Error("not installed")
	}
	// Reinstall replaces the block instead of appending a second one.
	in.Install(home, true, true)
	md, _ := os.ReadFile(filepath.Join(base, "CLAUDE.md"))
	if strings.Count(string(md), blockBegin) != 1 || !strings.HasPrefix(string(md), "# Mine\nkeep me\n\n") {
		t.Errorf("CLAUDE.md = %q", md)
	}
	removed, err := in.Uninstall(home)
	if err != nil || len(removed) != 4 {
		t.Errorf("removed = %v %v", removed, err)
	}
	md, _ = os.ReadFile(filepath.Join(base, "CLAUDE.md"))
	if string(md) != "# Mine\nkeep me\n" {
		t.Errorf("CLAUDE.md after uninstall = %q", md)
	}
	if _, err := os.Stat(filepath.Join(base, "skills", "handoff")); err == nil {
		t.Error("empty skill dir left behind")
	}
	if in.Installed(home) {
		t.Error("still installed")
	}
}

func TestOpencodeXDG(t *testing.T) {
	x := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", x)
	in, _ := Get("opencode")
	if _, _, err := in.Install("/nonexistent-home", false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(x, "opencode", "commands", "pickup.md")); err != nil {
		t.Error(err)
	}
	if _, err := Get("vim"); err == nil {
		t.Error("unknown integration accepted")
	}
}

func TestInstallNoTips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	in, _ := Get("claude")
	base := filepath.Join(home, ".claude")
	skill := filepath.Join(base, "skills", "tips", "SKILL.md")
	if _, _, err := in.Install(home, false, true); err != nil {
		t.Fatal(err)
	}
	written, removed, err := in.Install(home, false, false)
	if err != nil || len(written) != 3 || len(removed) != 1 || removed[0] != skill {
		t.Errorf("written = %v, removed = %v, %v", written, removed, err)
	}
	if _, err := os.Stat(filepath.Dir(skill)); err == nil {
		t.Error("tips skill left behind")
	}
	md, _ := os.ReadFile(filepath.Join(base, "CLAUDE.md"))
	if strings.Contains(string(md), "tips") || strings.Count(string(md), blockBegin) != 1 {
		t.Errorf("CLAUDE.md = %q", md)
	}
	// A foreign tips skill is not removed.
	os.MkdirAll(filepath.Dir(skill), 0o755)
	os.WriteFile(skill, []byte("mine"), 0o644)
	in.Install(home, false, false)
	if data, _ := os.ReadFile(skill); string(data) != "mine" {
		t.Error("foreign tips skill removed")
	}
	os.Remove(skill)
	in.Install(home, false, true)
	if _, err := os.Stat(skill); err != nil {
		t.Error("tips skill not restored")
	}
}
