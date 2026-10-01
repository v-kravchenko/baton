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
			data, err := Read(f.Src)
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
		if _, err := Read(in.Block.Src); err != nil {
			t.Errorf("%s: %v", in.Block.Src, err)
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

	written, err := in.Install(home, false)
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
	in.Install(home, true)
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
	if _, err := in.Install("/nonexistent-home", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(x, "opencode", "commands", "pickup.md")); err != nil {
		t.Error(err)
	}
	if _, err := Get("vim"); err == nil {
		t.Error("unknown integration accepted")
	}
}
