package tips

import (
	"os"
	"strings"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	return &Store{Root: t.TempDir(), LockDir: t.TempDir(), Now: func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }}
}

func TestStem(t *testing.T) {
	for in, want := range map[string]string{
		"locking": "lock", "locks": "lock", "libraries": "library", "go": "go",
		"конфігурації": "конфігураці", "конфігурація": "конфігураці", "помилки": "помилк", "помилкою": "помилк",
	} {
		if got := Stem(in); got != want {
			t.Errorf("Stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Flock fails on NFS!":             "flock-fails-on-nfs",
		"Шлях до конфігу":                 "shliakh-do-konfihu",
		"":                                "tip",
		strings.Repeat("abcdefghij ", 10): "abcdefghij-abcdefghij-abcdefghij-abcdefghij",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewSearchStatuses(t *testing.T) {
	s := newStore(t)
	body := "Tip: use LOCK_EX.\nWhy: x.\nVerify: run it.\n"
	a, warns, err := s.New(NewInput{Scope: "p", Body: "---\ntitle: Flock fails on NFS\nkeywords: [flock, NFS, file locking]\n---\n" + body, Project: "p", Commit: "abc"})
	if err != nil || len(warns) != 0 {
		t.Fatalf("%v %v", err, warns)
	}
	if a.ID != "flock-fails-on-nfs" || a.Status != Active || strings.Join(a.Source, " ") != "p abc 2026-10-01" {
		t.Errorf("tip = %+v", a)
	}
	b, _, _ := s.New(NewInput{Scope: "global", Body: "Tip: quote args.\n", Title: "Flock fails on NFS", When: "running claude.cmd on windows"}, "p")
	if b.ID != "flock-fails-on-nfs-2" {
		t.Errorf("dedupe id = %s", b.ID)
	}
	if _, _, err := s.New(NewInput{Scope: "p", Body: "x", Origin: "guess", Title: "t"}); err == nil {
		t.Error("bad origin accepted")
	}

	all, _ := s.List("p", "global")
	res := Search(all, "locking files", Options{})
	if len(res) != 1 || res[0].Tip.ID != a.ID {
		t.Fatalf("search = %+v", res)
	}
	res = Search(all, "claude.cmd windows quoting", Options{})
	if len(res) != 1 || res[0].Tip.ID != b.ID {
		t.Errorf("search 2 = %+v", res)
	}
	// Error mode drops noise and needs two hits.
	if r := Search(all, "error: flock 0x7f3a9c1d 12345", Options{Error: true}); len(r) != 0 {
		t.Errorf("error search with one hit = %+v", r)
	}
	if r := Search(all, "flock: NFS lock failed at 0x7f3a9c1d", Options{Error: true}); len(r) != 2 || r[0].Tip.ID != a.ID {
		t.Errorf("error search = %+v", r)
	}

	if err := s.SetRefuted(a, "works now"); err != nil {
		t.Fatal(err)
	}
	all, _ = s.List("p", "global")
	if r := Search(all, "flock", Options{}); len(r) != 1 || r[0].Tip.ID != b.ID {
		t.Errorf("refuted still found: %+v", r)
	}
	got, _ := s.Get(a.ID, "p")
	if got.Status != Refuted || !strings.Contains(got.Body, "Refuted (2026-10-01): works now") {
		t.Errorf("refuted tip = %+v", got)
	}
	s.SetVerified(got)
	if err := s.Supersede(b, got); err != nil {
		t.Fatal(err)
	}
	b2, _ := s.Get(b.ID, "global")
	if b2.Status != Superseded || b2.FM.Get("superseded_by") != a.ID {
		t.Errorf("superseded = %+v", b2)
	}
	m, err := s.Move(got, "global")
	if err != nil || m.Scope != "global" {
		t.Fatalf("move: %+v %v", m, err)
	}
	if _, err := s.Get(a.ID, "p"); err == nil {
		t.Error("moved tip still in project")
	}
}

func TestObsidianEditedTip(t *testing.T) {
	s := newStore(t)
	a, _, err := s.New(NewInput{Scope: "p", Title: "Gradle daemon dies", Body: "Tip: x.\nWhy: y.\nVerify: z.\n"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(a.Path)
	edited := strings.Replace(string(data), "origin:", "keywords:\n  - kotlin daemon\n  - oom\ntags:\n  - android\norigin:", 1)
	if err := os.WriteFile(a.Path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := s.List("p")
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v %d", err, len(all))
	}
	if res := Search(all, "kotlin oom", Options{}); len(res) != 1 {
		t.Errorf("block-list keywords not searched: %+v", res)
	}
	if err := s.SetVerified(all[0]); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(a.Path)
	if !strings.Contains(string(got), "keywords:\n  - kotlin daemon\n  - oom\ntags:\n  - android\n") ||
		!strings.Contains(string(got), "status: verified\n") {
		t.Errorf("rewrite lost fields:\n%s", got)
	}
}
