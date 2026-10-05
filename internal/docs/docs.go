// Package docs stores longer, stable documents of a task (plans, decisions,
// agreements) attached to its handoff. Docs live in
// <root>/<project>/docs/<task>/<id>.md and follow the task through done,
// restore and rename. There is no history and no status: a doc is edited in
// place or deleted.
package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/v-kravchenko/baton/internal/fsutil"
	"github.com/v-kravchenko/baton/internal/store"
	"github.com/v-kravchenko/baton/internal/tips"
)

// MaxChars is the body size budget: New and Edit warn above it.
const MaxChars = 8000

// Doc is one doc file.
type Doc struct {
	ID      string
	Project string
	Task    string
	Path    string
	FM      *store.Frontmatter
	Body    string
	Title   string
	Created time.Time
	Updated time.Time
	Version string // hash of the file; EditInput.Expect compares it
}

// Store is the docs view of a handoff root.
type Store struct {
	Root    string
	LockDir string
	Now     func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) dir(p, t string) string { return filepath.Join(s.Root, p, "docs", t) }

func (s *Store) lock() (func(), error) {
	return fsutil.Lock(filepath.Join(s.LockDir, "locks", "docs.lock"))
}

// Read parses a doc file.
func Read(path, project, task string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, body, err := store.Split(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	d := &Doc{ID: strings.TrimSuffix(filepath.Base(path), ".md"), Project: project, Task: task, Path: path, FM: fm, Body: body,
		Version: hex.EncodeToString(sum[:8])}
	d.load()
	if d.Updated.IsZero() {
		if st, err := os.Stat(path); err == nil {
			d.Updated = st.ModTime()
		}
	}
	if d.Created.IsZero() {
		d.Created = d.Updated
	}
	return d, nil
}

func (d *Doc) load() {
	fm := d.FM
	d.Title = fm.Get("title")
	d.Created, _ = time.Parse(time.RFC3339, fm.Get("created"))
	d.Updated, _ = time.Parse(time.RFC3339, fm.Get("updated"))
}

// List returns the docs of a task, newest update first. An empty task lists
// the docs of every task of the project.
func (s *Store) List(p, t string) ([]*Doc, error) {
	tasks := []string{t}
	if t == "" {
		ents, err := os.ReadDir(filepath.Join(s.Root, p, "docs"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		tasks = nil
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				tasks = append(tasks, e.Name())
			}
		}
	}
	var out []*Doc
	for _, task := range tasks {
		ents, err := os.ReadDir(s.dir(p, task))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return out, err
		}
		for _, e := range ents {
			n := e.Name()
			if e.IsDir() || strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") || strings.Contains(n, ".sync-conflict-") {
				continue
			}
			d, err := Read(filepath.Join(s.dir(p, task), n), p, task)
			if err != nil {
				return out, err
			}
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, nil
}

func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\`) && !strings.HasPrefix(id, ".")
}

// Get finds a doc of a task by id.
func (s *Store) Get(p, t, id string) (*Doc, error) {
	if !validID(id) || !validID(t) {
		return nil, fmt.Errorf("invalid doc id %q", id)
	}
	d, err := Read(filepath.Join(s.dir(p, t), id+".md"), p, t)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return nil, fmt.Errorf("no doc %q in @%s", id, t)
}

// NewInput describes a doc to create.
type NewInput struct {
	Project string
	Task    string
	ID      string // optional; else a slug of the title
	Body    string // may start with frontmatter (title)
	Title   string
}

// New writes a doc of an existing task and returns it with size warnings. A
// [[name]] in a handoff resolves to a task first, so the id must not be a task
// or tip name of the project: an explicit ID that is
// taken fails, a slug gets -2, -3...
func (s *Store) New(in NewInput) (*Doc, []string, error) {
	fm, body, err := store.Split([]byte(in.Body))
	if err != nil {
		return nil, nil, err
	}
	pick := func(flag, key, def string) string {
		if flag != "" {
			return flag
		}
		if v := fm.Get(key); v != "" {
			return v
		}
		return def
	}
	title := strings.TrimSpace(pick(in.Title, "title", ""))
	if title == "" {
		return nil, nil, fmt.Errorf("doc needs a title (--title or title: in frontmatter)")
	}
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if body == "" {
		return nil, nil, fmt.Errorf("empty doc body (pass it on stdin)")
	}
	if in.Project == "" || !validID(in.Task) {
		return nil, nil, fmt.Errorf("doc needs a project and a task")
	}
	if !s.taskExists(in.Project, in.Task) {
		return nil, nil, fmt.Errorf("no task %q in project %s", in.Task, in.Project)
	}

	// Fields of other tools (Obsidian tags, ...) passed on stdin are kept.
	out := fm.Clone()
	now := s.now().Format(time.RFC3339)
	out.Set("title", title)
	out.Set("created", now)
	out.Set("updated", now)

	unlock, err := s.lock()
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	id := in.ID
	if id != "" {
		if !validID(id) {
			return nil, nil, fmt.Errorf("invalid doc id %q", id)
		}
		if what := s.taken(in.Project, in.Task, id); what != "" {
			return nil, nil, fmt.Errorf("%s already has a %s %q", in.Project, what, id)
		}
	} else {
		base := tips.Words(title)
		if base == "" {
			base = "doc"
		}
		id = base
		for i := 2; s.taken(in.Project, in.Task, id) != ""; i++ {
			id = fmt.Sprintf("%s-%d", base, i)
		}
	}
	path := filepath.Join(s.dir(in.Project, in.Task), id+".md")
	if err := fsutil.WriteFileAtomic(path, store.Compose(out, body+"\n"), 0o644); err != nil {
		return nil, nil, err
	}
	d, err := Read(path, in.Project, in.Task)
	return d, budget(body), err
}

// taskExists reports whether t is an active or archived task of p.
func (s *Store) taskExists(p, t string) bool {
	for _, dir := range []string{"tasks", "archive"} {
		if _, err := os.Stat(filepath.Join(s.Root, p, dir, t+".md")); err == nil {
			return true
		}
	}
	return false
}

// taken names what already uses id ("doc" of the task, "task", "tip") or "".
func (s *Store) taken(p, t, id string) string {
	if _, err := os.Stat(filepath.Join(s.dir(p, t), id+".md")); err == nil {
		return "doc"
	}
	for _, dir := range []string{"tasks", "archive"} {
		if _, err := os.Stat(filepath.Join(s.Root, p, dir, id+".md")); err == nil {
			return "task"
		}
	}
	if _, err := os.Stat(filepath.Join(s.Root, p, "tips", id+".md")); err == nil {
		return "tip"
	}
	return ""
}

func budget(body string) []string {
	if n := utf8.RuneCountInString(body); n > MaxChars {
		return []string{fmt.Sprintf("doc body is %d characters, over the %d budget; split it", n, MaxChars)}
	}
	return nil
}

// EditInput replaces the title and body of a doc.
type EditInput struct {
	Title  string
	Body   string
	Expect string // Doc.Version the caller read; empty skips the check
}

// Edit rewrites d with in, keeping the id, created and other fields. It fails
// with store.ErrConflict when the file changed since in.Expect.
func (s *Store) Edit(d *Doc, in EditInput) (*Doc, []string, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, nil, fmt.Errorf("doc needs a title")
	}
	body := strings.TrimSpace(strings.ReplaceAll(in.Body, "\r\n", "\n"))
	if body == "" {
		return nil, nil, fmt.Errorf("empty doc body")
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	cur, err := Read(d.Path, d.Project, d.Task)
	if err != nil {
		return nil, nil, err
	}
	if in.Expect != "" && cur.Version != in.Expect {
		return nil, nil, fmt.Errorf("%w: doc %q changed since it was opened", store.ErrConflict, d.ID)
	}
	fm := cur.FM.Clone()
	fm.Set("title", title)
	fm.Set("updated", s.now().Format(time.RFC3339))
	if err := fsutil.WriteFileAtomic(cur.Path, store.Compose(fm, body+"\n"), 0o644); err != nil {
		return nil, nil, err
	}
	out, err := Read(cur.Path, cur.Project, cur.Task)
	return out, budget(body), err
}

// Delete removes a doc file.
func (s *Store) Delete(d *Doc) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return os.Remove(d.Path)
}
