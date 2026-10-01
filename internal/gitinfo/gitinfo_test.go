package gitinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, file string) {
	os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644)
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-qm", file)
}

func TestStale(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if _, ok := Read(dir); ok {
		t.Fatal("temp dir is a repo")
	}
	if l := Stale(dir, "main", "abc").Lines(); len(l) != 1 || !strings.Contains(l[0], "not a git repository") {
		t.Errorf("no repo lines = %v", l)
	}
	run(t, dir, "init", "-q", "-b", "main")
	commit(t, dir, "a")
	info, ok := Read(dir)
	if !ok || info.Branch != "main" || info.Commit == "" {
		t.Fatalf("info = %+v %v", info, ok)
	}
	if s := Stale(dir, info.Branch, info.Commit); !s.Fresh() {
		t.Errorf("fresh report = %+v", s)
	}
	commit(t, dir, "b")
	os.WriteFile(filepath.Join(dir, "c"), []byte("c"), 0o644)
	s := Stale(dir, info.Branch, info.Commit)
	if s.NewCommits != 1 || len(s.Log) != 1 || s.Dirty != 1 || s.ChangedFiles != 1 || s.Diverged {
		t.Errorf("report = %+v", s)
	}
	out := filepath.Join(t.TempDir(), "pwned")
	if s := Stale(dir, "main", "--output="+out); !s.CommitMissing {
		t.Errorf("option as commit = %+v", s)
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("git ran the commit value as an option")
	}
	if s := Stale(dir, "dev", "deadbee"); !s.CommitMissing || !s.BranchChanged {
		t.Errorf("missing report = %+v", s)
	}
	run(t, dir, "checkout", "-qb", "side", info.Commit)
	commit(t, dir, "d")
	side, _ := Read(dir)
	run(t, dir, "checkout", "-q", "main")
	if s := Stale(dir, "main", side.Commit); !s.Diverged {
		t.Errorf("diverged report = %+v", s)
	}
}
