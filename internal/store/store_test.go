package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProjectKey(t *testing.T) {
	for in, want := range map[string]string{"/a/My Repo": "my-repo", "/a/api.v2_x": "api.v2_x", "/a/Проєкт": "------"} {
		got, err := ProjectKey(in)
		if in == "/a/Проєкт" {
			if err == nil {
				t.Errorf("%s accepted as %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%s = %q %v", in, got, err)
		}
	}
	if _, err := ProjectKey("/x/Global"); err == nil {
		t.Error("global accepted")
	}
	if _, err := ProjectKey("/"); err == nil {
		t.Error("/ accepted")
	}
	if _, err := ProjectKey("/x/CON"); err == nil {
		t.Error("con accepted")
	}
}

func TestTaskName(t *testing.T) {
	for _, ok := range []string{"@Api-Refactor", "console", "com10", "lpt"} {
		if _, err := TaskName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if n, err := TaskName("@Api-Refactor"); err != nil || n != "api-refactor" {
		t.Errorf("%q %v", n, err)
	}
	for _, bad := range []string{"", "a/b", "../x", ".x", "a b", "nul", "com1", "aux.x"} {
		if _, err := TaskName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func newStore(t *testing.T) (*Store, *time.Time) {
	now := time.Date(2026, 10, 1, 10, 0, 0, 0, time.FixedZone("", 3*3600))
	s := &Store{Root: t.TempDir(), LockDir: t.TempDir(), Keep: 2}
	s.Now = func() time.Time { return now }
	return s, &now
}

func TestSaveRotateKeep(t *testing.T) {
	s, now := newStore(t)
	for i := 0; i < 4; i++ {
		res, err := s.Save("p", "t", SaveInput{Body: "# Head\n\n## Goal\nx\n", Branch: "main"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Handoff.Title != "Head" || res.Handoff.Branch != "main" {
			t.Errorf("handoff = %+v", res.Handoff)
		}
		if i > 0 && res.Rotated == "" {
			t.Errorf("save %d did not rotate", i)
		}
		*now = now.Add(time.Hour)
	}
	hs, _ := s.History("p", "t")
	if len(hs) != 2 {
		t.Fatalf("history len = %d", len(hs))
	}
	if filepath.Base(hs[0].Path) != "20261001T090000Z.md" || filepath.Base(hs[1].Path) != "20261001T080000Z.md" {
		t.Errorf("history = %s, %s", hs[0].Path, hs[1].Path)
	}
	// Same-second saves do not overwrite history.
	*now = now.Add(-time.Hour)
	s.Keep = 10
	s.Save("p", "t", SaveInput{Body: "b"})
	s.Save("p", "t", SaveInput{Body: "b"})
	hs, _ = s.History("p", "t")
	if len(hs) != 4 {
		t.Errorf("history len after same-second saves = %d", len(hs))
	}
}

func TestSaveStripsFrontmatterAndKeepsTitle(t *testing.T) {
	s, _ := newStore(t)
	s.Save("p", "t", SaveInput{Body: "---\ntitle: From FM\n---\nbody\n", From: "parent"})
	h, _ := s.Get("p", "t")
	if h.Title != "From FM" || h.From != "parent" || strings.Contains(h.Body, "---") {
		t.Errorf("%+v", h)
	}
	s.Save("p", "t", SaveInput{Body: "new body"})
	h, _ = s.Get("p", "t")
	if h.Title != "From FM" || h.From != "parent" {
		t.Errorf("title/from not carried over: %+v", h)
	}
	if _, err := s.Save("p", "t", SaveInput{Body: "  \n"}); err == nil {
		t.Error("empty body accepted")
	}
	if _, err := s.Save("p", "t", SaveInput{Body: "x", From: "t"}); err == nil {
		t.Error("self fork accepted")
	}
}

func TestDoneRestoreRenameForks(t *testing.T) {
	s, _ := newStore(t)
	s.Save("p", "a", SaveInput{Body: "a"})
	s.Save("p", "a", SaveInput{Body: "a2"})
	s.Save("p", "b", SaveInput{Body: "b", From: "a"})
	if f := s.Forks("p", "a"); !reflect.DeepEqual(f, []string{"b"}) {
		t.Errorf("forks = %v", f)
	}
	if _, err := s.Done("p", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Done("p", "a"); err == nil {
		t.Error("double done accepted")
	}
	h, _ := s.Get("p", "a")
	if !h.Archived {
		t.Error("not archived")
	}
	upd, err := s.Rename("p", "a", "c")
	if err != nil || !reflect.DeepEqual(upd, []string{"b"}) {
		t.Fatalf("rename: %v %v", upd, err)
	}
	if hs, _ := s.History("p", "c"); len(hs) != 1 {
		t.Errorf("history not moved: %d", len(hs))
	}
	if b, _ := s.Get("p", "b"); b.From != "c" {
		t.Errorf("fork from = %q", b.From)
	}
	if _, err := s.Rename("p", "b", "c"); err == nil {
		t.Error("rename onto existing accepted")
	}
	if _, err := s.Restore("p", "c"); err != nil {
		t.Fatal(err)
	}
	if ts, _ := s.Tasks("p"); len(ts) != 2 {
		t.Errorf("tasks = %d", len(ts))
	}
	// Saving an archived task restores it.
	s.Done("p", "b")
	res, err := s.Save("p", "b", SaveInput{Body: "again"})
	if err != nil || !res.Restored {
		t.Errorf("save archived: %+v %v", res, err)
	}
}

func TestConflicts(t *testing.T) {
	s, _ := newStore(t)
	s.Save("p", "a", SaveInput{Body: "a"})
	f := filepath.Join(s.Root, "p", "tasks", "a.sync-conflict-20261001-120000-ABC.md")
	os.WriteFile(f, []byte("x"), 0o644)
	if c := s.Conflicts("p"); !reflect.DeepEqual(c, []string{"p/tasks/a.sync-conflict-20261001-120000-ABC.md"}) {
		t.Errorf("conflicts = %v", c)
	}
	if ts, _ := s.Tasks("p"); len(ts) != 1 {
		t.Errorf("conflict counted as task: %d", len(ts))
	}
}

func TestMissingSections(t *testing.T) {
	m := MissingSections(Template)
	if len(m) != 0 {
		t.Errorf("template misses %v", m)
	}
	if m := MissingSections("## goal\n## State\n"); len(m) != 1 || m[0] != "Next steps" {
		t.Errorf("missing = %v", m)
	}
	if g := Section(Template, "context"); !strings.HasPrefix(g, "Optional.") {
		t.Errorf("section = %q", g)
	}
}

func TestSizeWarnings(t *testing.T) {
	if w := SizeWarnings(Template); len(w) != 0 {
		t.Errorf("template: %v", w)
	}
	// Cyrillic counts characters, not bytes; fenced code is not counted.
	line := strings.Repeat("ж", MaxLine)
	if w := SizeWarnings(line + "\n```\n" + strings.Repeat("x", 5000) + "\n```\n"); len(w) != 0 {
		t.Errorf("within budget: %v", w)
	}
	w := SizeWarnings(strings.Repeat(line+"ж\n", 15))
	if len(w) != 2 || !strings.Contains(w[0], "3031 characters") || !strings.HasPrefix(w[1], "15 lines") {
		t.Errorf("over budget: %v", w)
	}
}

func TestSaveKeepsForeignFields(t *testing.T) {
	s, _ := newStore(t)
	s.Save("p", "t", SaveInput{Body: "a", Branch: "main", Commit: "abc"})
	path := s.taskPath("p", "t")
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), "---\n\n", "tags:\n  - work\naliases: [x]\n---\n\n", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save("p", "t", SaveInput{Body: "b"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "---\ntitle: \"t\"\ncreated: 2026-10-01T10:00:00+03:00\ntags:\n  - work\naliases: [x]\n---\n\nb\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
