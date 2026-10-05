package docs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/v-kravchenko/baton/internal/store"
)

var day = time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

func newStore(t *testing.T) *Store {
	return &Store{Root: t.TempDir(), LockDir: t.TempDir(), Now: func() time.Time { return day }}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\ntitle: x\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTask(t *testing.T, s *Store, p, task string) {
	t.Helper()
	touch(t, filepath.Join(s.Root, p, "tasks", task+".md"))
}

func TestNewGetList(t *testing.T) {
	s := newStore(t)
	if _, _, err := s.New(NewInput{Project: "p", Task: "a", Title: "t", Body: "x"}); err == nil {
		t.Fatal("doc of a missing task accepted")
	}
	newTask(t, s, "p", "a")
	newTask(t, s, "p", "b")
	d, warns, err := s.New(NewInput{Project: "p", Task: "a", Body: "---\ntitle: Workflow міграції\ntags: [a]\n---\n1. Оцінити.\n"})
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %v", err, warns)
	}
	if d.ID != "workflow-mihratsii" || d.Task != "a" || !d.Created.Equal(day) {
		t.Errorf("doc = %+v", d)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "p", "docs", "a", d.ID+".md")); err != nil {
		t.Error(err)
	}
	if strings.Join(d.FM.GetList("tags"), ",") != "a" || d.Body != "1. Оцінити.\n" {
		t.Errorf("fm/body = %v %q", d.FM.Fields, d.Body)
	}
	if _, err := s.Get("p", "a", "workflow-mihratsii"); err != nil {
		t.Error(err)
	}
	if _, err := s.Get("p", "b", "workflow-mihratsii"); err == nil {
		t.Error("doc found in another task")
	}
	if _, err := s.Get("p", "a", "../x"); err == nil {
		t.Error("bad id accepted")
	}

	// The same id is fine in another task; slug ids skip tasks, tips and
	// docs of the task; explicit ids fail.
	if _, _, err := s.New(NewInput{Project: "p", Task: "b", ID: d.ID, Title: "t", Body: "x"}); err != nil {
		t.Errorf("same id in another task: %v", err)
	}
	touch(t, filepath.Join(s.Root, "p", "tasks", "plan.md"))
	touch(t, filepath.Join(s.Root, "p", "tips", "plan-2.md"))
	d2, _, err := s.New(NewInput{Project: "p", Task: "a", Title: "Plan", Body: "x"})
	if err != nil || d2.ID != "plan-3" {
		t.Errorf("id = %v %v", d2, err)
	}
	if _, _, err := s.New(NewInput{Project: "p", Task: "a", ID: "plan", Title: "t", Body: "x"}); err == nil || !strings.Contains(err.Error(), "task") {
		t.Errorf("taken id: %v", err)
	}
	for _, in := range []NewInput{
		{Project: "p", Task: "a", Body: "x"},
		{Project: "p", Task: "a", Title: "t"},
	} {
		if _, _, err := s.New(in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	_, warns, _ = s.New(NewInput{Project: "p", Task: "a", Title: "big", Body: strings.Repeat("ж", MaxChars+1)})
	if len(warns) != 1 {
		t.Errorf("warns = %v", warns)
	}

	if ds, err := s.List("p", "a"); err != nil || len(ds) != 3 {
		t.Errorf("list a = %v %v", ds, err)
	}
	if ds, err := s.List("p", ""); err != nil || len(ds) != 4 {
		t.Errorf("list project = %v %v", ds, err)
	}
}

func TestEditConflict(t *testing.T) {
	s := newStore(t)
	newTask(t, s, "p", "a")
	d, _, _ := s.New(NewInput{Project: "p", Task: "a", Title: "Rules", Body: "a"})
	s.Now = func() time.Time { return day.Add(time.Hour) }
	e, _, err := s.Edit(d, EditInput{Title: "Rules 2", Body: "b", Expect: d.Version})
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Rules 2" || e.Body != "b\n" || !e.Created.Equal(day) || !e.Updated.Equal(day.Add(time.Hour)) {
		t.Errorf("edited = %+v", e)
	}
	if _, _, err := s.Edit(d, EditInput{Title: "x", Body: "c", Expect: d.Version}); !errors.Is(err, store.ErrConflict) {
		t.Errorf("stale edit: %v", err)
	}
	if err := s.Delete(e); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("p", "a", e.ID); err == nil {
		t.Error("deleted doc found")
	}
}

// Docs follow their task through done, restore and rename.
func TestDocsFollowTask(t *testing.T) {
	s := newStore(t)
	st := &store.Store{Root: s.Root, LockDir: s.LockDir, Keep: 3, Now: s.Now}
	if _, err := st.Save("p", "a", store.SaveInput{Title: "a", Body: "## Goal\nx\n"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.New(NewInput{Project: "p", Task: "a", ID: "flow", Title: "Flow", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Done("p", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("p", "a", "flow"); err != nil {
		t.Errorf("after done: %v", err)
	}
	if _, _, err := s.New(NewInput{Project: "p", Task: "a", Title: "More", Body: "x"}); err != nil {
		t.Errorf("doc of an archived task: %v", err)
	}
	if _, err := st.Rename("p", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if ds, _ := s.List("p", "b"); len(ds) != 2 {
		t.Errorf("after rename: %v", ds)
	}
	if ds, _ := s.List("p", "a"); len(ds) != 0 {
		t.Errorf("old name keeps docs: %v", ds)
	}
}
