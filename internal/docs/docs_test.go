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

func TestNewGetList(t *testing.T) {
	s := newStore(t)
	d, warns, err := s.New(NewInput{Scope: "p", Body: "---\ntitle: Workflow міграції\ntags: [a]\n---\n1. Оцінити.\n"})
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %v", err, warns)
	}
	if d.ID != "workflow-mihratsii" || !d.Created.Equal(day) {
		t.Errorf("doc = %+v", d)
	}
	if strings.Join(d.FM.GetList("tags"), ",") != "a" || d.Body != "1. Оцінити.\n" {
		t.Errorf("fm/body = %v %q", d.FM.Fields, d.Body)
	}
	if _, err := s.Get("workflow-mihratsii", "q", "p"); err != nil {
		t.Error(err)
	}
	if _, err := s.Get("../x", "p"); err == nil {
		t.Error("bad id accepted")
	}

	// Slug ids skip tasks, tips and docs of the scope; explicit ids fail.
	touch(t, filepath.Join(s.Root, "p", "tasks", "plan.md"))
	touch(t, filepath.Join(s.Root, "p", "tips", "plan-2.md"))
	d2, _, err := s.New(NewInput{Scope: "p", Title: "Plan", Body: "x"})
	if err != nil || d2.ID != "plan-3" {
		t.Errorf("id = %v %v", d2, err)
	}
	if _, _, err := s.New(NewInput{Scope: "p", ID: "plan", Title: "t", Body: "x"}); err == nil || !strings.Contains(err.Error(), "task") {
		t.Errorf("taken id: %v", err)
	}
	for _, in := range []NewInput{
		{Scope: "p", Body: "x"},
		{Scope: "p", Title: "t"},
	} {
		if _, _, err := s.New(in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	_, warns, _ = s.New(NewInput{Scope: "p", Title: "big", Body: strings.Repeat("ж", MaxChars+1)})
	if len(warns) != 1 {
		t.Errorf("warns = %v", warns)
	}

	if ds, err := s.List("p"); err != nil || len(ds) != 3 {
		t.Errorf("list = %v %v", ds, err)
	}
}

func TestEditConflict(t *testing.T) {
	s := newStore(t)
	d, _, _ := s.New(NewInput{Scope: "p", Title: "Rules", Body: "a"})
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
	if _, err := s.Get(e.ID, "p"); err == nil {
		t.Error("deleted doc found")
	}
}

func TestRefsLinkedBacklinks(t *testing.T) {
	if got := strings.Join(Refs("see [[a]], [[b|B]] and [[a]]; not [[]] or [x]"), ","); got != "a,b" {
		t.Errorf("Refs = %s", got)
	}
	s := newStore(t)
	d, _, _ := s.New(NewInput{Scope: "p", ID: "flow", Title: "Flow", Body: "x"})
	if got := s.Linked("[[nope]] [[flow|the flow]]", "p"); len(got) != 1 || got[0].ID != d.ID {
		t.Errorf("Linked = %v", got)
	}
	st := &store.Store{Root: s.Root, LockDir: s.LockDir, Keep: 3, Now: s.Now}
	for task, body := range map[string]string{"a": "uses [[flow]]", "b": "no link"} {
		if _, err := st.Save("p", task, store.SaveInput{Title: task, Body: "## Goal\n" + body + "\n"}); err != nil {
			t.Fatal(err)
		}
	}
	if got := Backlinks(st, "p", "flow"); strings.Join(got, ",") != "a" {
		t.Errorf("Backlinks = %v", got)
	}
}
