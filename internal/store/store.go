// Package store keeps handoffs under <root>/<project>/{tasks,archive,history}.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

// Global is the reserved root-level name for global tips.
const Global = "global"

// HistoryStamp is the UTC layout of history file names.
const HistoryStamp = "20060102T150405Z"

// ProjectKey derives the project key from a directory name: lower case, every
// rune outside a-z0-9._- replaced with '-'.
func ProjectKey(dir string) (string, error) {
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "", fmt.Errorf("cannot derive a project key from %q", dir)
	}
	key := sanitize(base)
	if strings.Trim(key, ".-") == "" {
		return "", fmt.Errorf("cannot derive a project key from %q", dir)
	}
	if key == Global {
		return "", fmt.Errorf("directory name %q is reserved by baton; rename the directory", base)
	}
	if winReserved(key) {
		return "", fmt.Errorf("directory name %q is a reserved device name on Windows; rename the directory", base)
	}
	return key, nil
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

// TaskName validates and normalizes a task name (a leading @ is dropped).
func TaskName(s string) (string, error) {
	s = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "@"))
	if s == "" {
		return "", fmt.Errorf("empty task name")
	}
	if s[0] == '.' || s[0] == '-' || sanitize(s) != s || strings.Contains(s, "..") {
		return "", fmt.Errorf("invalid task name %q: use a-z, 0-9, '.', '_', '-'", s)
	}
	if winReserved(s) {
		return "", fmt.Errorf("invalid task name %q: a reserved device name on Windows", s)
	}
	return s, nil
}

// winReserved reports names Windows treats as devices with any extension
// (con, con.md, com1, ...); they are refused on every OS so data syncs.
func winReserved(s string) bool {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "con", "prn", "aux", "nul":
		return true
	}
	return len(s) == 4 && (strings.HasPrefix(s, "com") || strings.HasPrefix(s, "lpt")) && s[3] >= '0' && s[3] <= '9'
}

// Store is a handoff root.
type Store struct {
	Root    string
	LockDir string // per-machine directory for lock files
	Keep    int    // history entries kept per task
	Now     func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handoff is one parsed handoff file.
type Handoff struct {
	Project  string
	Task     string
	Path     string
	Archived bool
	FM       *Frontmatter
	Body     string
	Title    string
	Created  time.Time
	From     string
	Branch   string
	Commit   string
}

// ProjectDir returns <root>/<project>.
func (s *Store) ProjectDir(p string) string { return filepath.Join(s.Root, p) }

func (s *Store) taskPath(p, t string) string {
	return filepath.Join(s.Root, p, "tasks", t+".md")
}
func (s *Store) archivePath(p, t string) string {
	return filepath.Join(s.Root, p, "archive", t+".md")
}
func (s *Store) historyDir(p, t string) string { return filepath.Join(s.Root, p, "history", t) }

// Exists reports whether the project has a directory in root.
func (s *Store) Exists(p string) bool {
	st, err := os.Stat(s.ProjectDir(p))
	return err == nil && st.IsDir()
}

// Lock serializes changes to one project on this machine.
func (s *Store) Lock(p string) (func(), error) {
	dir := s.LockDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "baton-locks")
	}
	return fsutil.Lock(filepath.Join(dir, "locks", p+".lock"))
}

// ReadFile parses a handoff document from path.
func ReadFile(path string) (*Handoff, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, body, err := Split(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	h := &Handoff{Path: path, FM: fm, Body: body, Task: strings.TrimSuffix(filepath.Base(path), ".md")}
	h.Title = fm.Get("title")
	h.From = fm.Get("from")
	h.Branch = fm.Get("branch")
	h.Commit = fm.Get("commit")
	if t, err := time.Parse(time.RFC3339, fm.Get("created")); err == nil {
		h.Created = t
	} else if st, err := os.Stat(path); err == nil {
		h.Created = st.ModTime()
	}
	return h, nil
}

func (s *Store) read(p, t string, archived bool) (*Handoff, error) {
	path := s.taskPath(p, t)
	if archived {
		path = s.archivePath(p, t)
	}
	h, err := ReadFile(path)
	if err != nil {
		return nil, err
	}
	h.Project, h.Task, h.Archived = p, t, archived
	return h, nil
}

// Get returns the active or archived handoff for a task, or fs.ErrNotExist.
func (s *Store) Get(p, t string) (*Handoff, error) {
	h, err := s.read(p, t, false)
	if errors.Is(err, fs.ErrNotExist) {
		h, err = s.read(p, t, true)
	}
	return h, err
}

func listMD(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".md") || strings.Contains(n, ".sync-conflict-") {
			continue
		}
		out = append(out, strings.TrimSuffix(n, ".md"))
	}
	sort.Strings(out)
	return out
}

func (s *Store) list(p string, archived bool) ([]*Handoff, error) {
	dir := filepath.Join(s.Root, p, "tasks")
	if archived {
		dir = filepath.Join(s.Root, p, "archive")
	}
	var out []*Handoff
	for _, t := range listMD(dir) {
		h, err := s.read(p, t, archived)
		if err != nil {
			return out, err
		}
		out = append(out, h)
	}
	// Most recently saved first.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// Tasks returns active handoffs, newest first.
func (s *Store) Tasks(p string) ([]*Handoff, error) { return s.list(p, false) }

// Archived returns archived handoffs, newest first.
func (s *Store) Archived(p string) ([]*Handoff, error) { return s.list(p, true) }

// Projects lists project directories in root (excluding global and hidden).
func (s *Store) Projects() []string {
	ents, err := os.ReadDir(s.Root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && e.Name() != Global && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// Forks returns tasks (active and archived) whose `from` is t.
func (s *Store) Forks(p, t string) []string {
	var out []string
	for _, archived := range []bool{false, true} {
		hs, _ := s.list(p, archived)
		for _, h := range hs {
			if h.From == t {
				out = append(out, h.Task)
			}
		}
	}
	sort.Strings(out)
	return out
}

// SaveInput is what `baton save` passes to the store.
type SaveInput struct {
	Body   string
	Title  string
	From   string
	Branch string
	Commit string
}

// SaveResult reports what Save did.
type SaveResult struct {
	Handoff  *Handoff
	Rotated  string   // previous handoff moved to history (path)
	Pruned   []string // history files removed by keep-N
	Restored bool     // the task was archived and became active again
	Missing  []string // template sections absent from the body
}

// Save writes a new handoff for task t, rotating the previous one to history.
func (s *Store) Save(p, t string, in SaveInput) (*SaveResult, error) {
	unlock, err := s.Lock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()

	res := &SaveResult{}
	body := in.Body
	var bodyFM *Frontmatter
	if fm, b, err := Split([]byte(body)); err == nil && len(fm.Fields) > 0 {
		bodyFM, body = fm, b
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("empty handoff body (pass it on stdin)")
	}

	prev, err := s.read(p, t, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if prev == nil {
		if a, err := s.read(p, t, true); err == nil {
			prev = a
			res.Restored = true
		}
	}

	title := in.Title
	if title == "" && bodyFM != nil {
		title = bodyFM.Get("title")
	}
	if title == "" && prev != nil {
		title = prev.Title
	}
	if title == "" {
		title = firstHeading(body)
	}
	if title == "" {
		title = t
	}
	from := in.From
	if from == "" && prev != nil {
		from = prev.From
	}
	if from == t {
		return nil, fmt.Errorf("task %q cannot be forked from itself", t)
	}

	// Fields added by other tools (Obsidian tags, aliases, ...) are kept.
	fm := &Frontmatter{}
	if prev != nil {
		fm = prev.FM.Clone()
	}
	fm.Set("title", title)
	fm.Set("created", s.now().Format(time.RFC3339))
	fm.Set("branch", in.Branch)
	fm.Set("commit", in.Commit)
	fm.Set("from", from)

	if prev != nil {
		dst, err := s.rotate(prev)
		if err != nil {
			return nil, err
		}
		res.Rotated = dst
	}
	path := s.taskPath(p, t)
	if err := fsutil.WriteFileAtomic(path, Compose(fm, body), 0o644); err != nil {
		return nil, err
	}
	res.Pruned = s.prune(p, t)
	res.Missing = MissingSections(body)
	res.Handoff, err = s.read(p, t, false)
	return res, err
}

// rotate moves a handoff into history/<task>/<created UTC>.md.
func (s *Store) rotate(h *Handoff) (string, error) {
	dir := s.historyDir(h.Project, h.Task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := h.Created.UTC().Format(HistoryStamp)
	dst := filepath.Join(dir, stamp+".md")
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); errors.Is(err, fs.ErrNotExist) {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s-%d.md", stamp, i))
	}
	return dst, fsutil.Rename(h.Path, dst)
}

// prune keeps the newest Keep history entries.
func (s *Store) prune(p, t string) []string {
	names := listMD(s.historyDir(p, t))
	var removed []string
	for len(names) > s.Keep {
		f := filepath.Join(s.historyDir(p, t), names[0]+".md")
		if os.Remove(f) == nil {
			removed = append(removed, f)
		}
		names = names[1:]
	}
	if len(listMD(s.historyDir(p, t))) == 0 {
		os.Remove(s.historyDir(p, t))
	}
	return removed
}

// History returns previous handoffs of t, newest first.
func (s *Store) History(p, t string) ([]*Handoff, error) {
	names := listMD(s.historyDir(p, t))
	out := make([]*Handoff, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		h, err := ReadFile(filepath.Join(s.historyDir(p, t), names[i]+".md"))
		if err != nil {
			return out, err
		}
		h.Project, h.Task = p, t
		out = append(out, h)
	}
	return out, nil
}

// Done archives an active task.
func (s *Store) Done(p, t string) (*Handoff, error) {
	unlock, err := s.Lock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()
	h, err := s.read(p, t, false)
	if errors.Is(err, fs.ErrNotExist) {
		if _, aerr := s.read(p, t, true); aerr == nil {
			return nil, fmt.Errorf("task %q is already archived", t)
		}
		return nil, fmt.Errorf("no active task %q in project %s", t, p)
	}
	if err != nil {
		return nil, err
	}
	if old, err := s.read(p, t, true); err == nil {
		if _, err := s.rotate(old); err != nil {
			return nil, err
		}
		s.prune(p, t)
	}
	dst := s.archivePath(p, t)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := fsutil.Rename(h.Path, dst); err != nil {
		return nil, err
	}
	return s.read(p, t, true)
}

// Restore makes an archived task active again.
func (s *Store) Restore(p, t string) (*Handoff, error) {
	unlock, err := s.Lock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()
	h, err := s.read(p, t, true)
	if errors.Is(err, fs.ErrNotExist) {
		if _, aerr := s.read(p, t, false); aerr == nil {
			return nil, fmt.Errorf("task %q is already active", t)
		}
		return nil, fmt.Errorf("no archived task %q in project %s", t, p)
	}
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.taskPath(p, t)); err == nil {
		return nil, fmt.Errorf("task %q exists both active and archived; resolve by hand", t)
	}
	dst := s.taskPath(p, t)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := fsutil.Rename(h.Path, dst); err != nil {
		return nil, err
	}
	return s.read(p, t, false)
}

// Rename renames a task (active or archived) with its history and updates
// `from` in its forks. It returns the forks that were updated.
func (s *Store) Rename(p, oldT, newT string) ([]string, error) {
	if oldT == newT {
		return nil, fmt.Errorf("old and new names are the same")
	}
	unlock, err := s.Lock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()
	h, err := s.Get(p, oldT)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no task %q in project %s", oldT, p)
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.Get(p, newT); err == nil {
		return nil, fmt.Errorf("task %q already exists", newT)
	}
	if _, err := os.Stat(s.historyDir(p, newT)); err == nil {
		return nil, fmt.Errorf("history for %q already exists: %s", newT, s.historyDir(p, newT))
	}
	dst := s.taskPath(p, newT)
	if h.Archived {
		dst = s.archivePath(p, newT)
	}
	if err := fsutil.Rename(h.Path, dst); err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.historyDir(p, oldT)); err == nil {
		if err := fsutil.Rename(s.historyDir(p, oldT), s.historyDir(p, newT)); err != nil {
			return nil, err
		}
	}
	var updated []string
	for _, archived := range []bool{false, true} {
		hs, _ := s.list(p, archived)
		for _, c := range hs {
			if c.From != oldT {
				continue
			}
			c.FM.Set("from", newT)
			if err := fsutil.WriteFileAtomic(c.Path, Compose(c.FM, c.Body), 0o644); err != nil {
				return updated, err
			}
			updated = append(updated, c.Task)
		}
	}
	return updated, nil
}

// Conflicts lists Syncthing conflict files under a project (paths relative
// to root).
func (s *Store) Conflicts(p string) []string {
	var out []string
	base := s.ProjectDir(p)
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.Contains(d.Name(), ".sync-conflict-") {
			if rel, err := filepath.Rel(s.Root, path); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return out
}

func firstHeading(body string) string {
	for _, l := range strings.Split(body, "\n") {
		if h, ok := strings.CutPrefix(l, "# "); ok {
			return strings.TrimSpace(h)
		}
	}
	return ""
}
