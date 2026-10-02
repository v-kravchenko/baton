// Package tips stores short, unverified hints with a Verify step and finds
// them by keywords. Tips live in <root>/<project>/tips and <root>/global/tips.
package tips

import (
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
)

// Statuses.
const (
	Active     = "active"
	Verified   = "verified"
	Refuted    = "refuted"
	Superseded = "superseded"
)

// Size budget of a tip: New warns above it.
const (
	MaxTitle    = 70
	MaxKeywords = 8
)

// Origins.
var Origins = []string{"session", "failure", "web", "user"}

// Tip is one tip file.
type Tip struct {
	ID       string
	Scope    string // project key or store.Global
	Path     string
	FM       *store.Frontmatter
	Body     string
	Title    string
	When     string
	Keywords []string
	Cites    []string
	Origin   string
	Source   []string // [project, commit, date]
	Status   string
	Env      []string
}

// Date returns the source date (or zero).
func (t *Tip) Date() time.Time {
	if len(t.Source) > 0 {
		if d, err := time.Parse("2006-01-02", t.Source[len(t.Source)-1]); err == nil {
			return d
		}
	}
	return time.Time{}
}

// Live reports whether the tip should be offered by default.
func (t *Tip) Live() bool { return t.Status != Refuted && t.Status != Superseded }

// Store is the tips view of a handoff root.
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

func (s *Store) dir(scope string) string { return filepath.Join(s.Root, scope, "tips") }

func (s *Store) lock() (func(), error) {
	return fsutil.Lock(filepath.Join(s.LockDir, "locks", "tips.lock"))
}

// Read parses a tip file.
func Read(path, scope string) (*Tip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fm, body, err := store.Split(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	t := &Tip{ID: strings.TrimSuffix(filepath.Base(path), ".md"), Scope: scope, Path: path, FM: fm, Body: body}
	t.load()
	return t, nil
}

func (t *Tip) load() {
	fm := t.FM
	t.Title = fm.Get("title")
	t.When = fm.Get("when")
	t.Keywords = fm.GetList("keywords")
	t.Cites = fm.GetList("cites")
	t.Origin = fm.Get("origin")
	t.Source = fm.GetList("source")
	t.Status = fm.Get("status")
	if t.Status == "" {
		t.Status = Active
	}
	t.Env = fm.GetList("env")
}

// List returns tips of the given scopes (project scopes first as passed).
func (s *Store) List(scopes ...string) ([]*Tip, error) {
	var out []*Tip
	for _, sc := range scopes {
		if sc == "" {
			continue
		}
		ents, err := os.ReadDir(s.dir(sc))
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
			t, err := Read(filepath.Join(s.dir(sc), n), sc)
			if err != nil {
				return out, err
			}
			out = append(out, t)
		}
	}
	return out, nil
}

// Get finds a tip by id in the scopes, in order.
func (s *Store) Get(id string, scopes ...string) (*Tip, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		return nil, fmt.Errorf("invalid tip id %q", id)
	}
	for _, sc := range scopes {
		if sc == "" {
			continue
		}
		t, err := Read(filepath.Join(s.dir(sc), id+".md"), sc)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no tip %q", id)
}

// NewInput describes a tip to create.
type NewInput struct {
	Scope    string
	Body     string // may start with frontmatter (title, when, keywords, ...)
	Title    string
	When     string
	Keywords []string
	Cites    []string
	Origin   string
	Env      []string
	Project  string // source project
	Commit   string // source commit
}

// New writes a tip and returns it. The id is a slug of the title, unique
// across the given lookup scopes.
func (s *Store) New(in NewInput, lookup ...string) (*Tip, []string, error) {
	fm, body, err := store.Split([]byte(in.Body))
	if err != nil {
		return nil, nil, err
	}
	pick := func(flag, key string) string {
		if flag != "" {
			return flag
		}
		return fm.Get(key)
	}
	pickList := func(flag []string, key string) []string {
		if len(flag) > 0 {
			return flag
		}
		return fm.GetList(key)
	}
	title := pick(in.Title, "title")
	if title == "" {
		return nil, nil, fmt.Errorf("tip needs a title (--title or title: in frontmatter)")
	}
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if body == "" {
		return nil, nil, fmt.Errorf("empty tip body (pass Tip:/Why:/Verify: on stdin)")
	}
	origin := pick(in.Origin, "origin")
	if origin == "" {
		origin = "session"
	}
	valid := false
	for _, o := range Origins {
		valid = valid || o == origin
	}
	if !valid {
		return nil, nil, fmt.Errorf("origin must be one of %s", strings.Join(Origins, ", "))
	}
	var warns []string
	for _, part := range []string{"Tip:", "Why:", "Verify:"} {
		if !strings.Contains(body, part) {
			warns = append(warns, "missing "+part)
		}
	}
	if n := utf8.RuneCountInString(title); n > MaxTitle {
		warns = append(warns, fmt.Sprintf("title is %d characters, budget %d", n, MaxTitle))
	}
	keywords := normalizeKeywords(pickList(in.Keywords, "keywords"))
	if len(keywords) > MaxKeywords {
		warns = append(warns, fmt.Sprintf("%d keywords, budget %d: keep the words an agent would search for", len(keywords), MaxKeywords))
	}
	commit := in.Commit
	if commit == "" {
		commit = "-"
	}
	project := in.Project
	if project == "" {
		project = "-"
	}

	out := &store.Frontmatter{}
	out.Set("title", title)
	out.Set("when", pick(in.When, "when"))
	out.SetList("keywords", keywords)
	out.SetList("cites", pickList(in.Cites, "cites"))
	out.Set("origin", origin)
	out.SetList("source", []string{project, commit, s.now().Format("2006-01-02")})
	out.Set("status", Active)
	out.SetList("env", pickList(in.Env, "env"))

	unlock, err := s.lock()
	if err != nil {
		return nil, nil, err
	}
	defer unlock()
	id := s.uniqueID(Slug(title), append([]string{in.Scope}, lookup...))
	path := filepath.Join(s.dir(in.Scope), id+".md")
	if err := fsutil.WriteFileAtomic(path, store.Compose(out, body+"\n"), 0o644); err != nil {
		return nil, nil, err
	}
	t, err := Read(path, in.Scope)
	return t, warns, err
}

func normalizeKeywords(ks []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, k := range ks {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func (s *Store) uniqueID(base string, scopes []string) string {
	id := base
	for i := 2; ; i++ {
		taken := false
		for _, sc := range scopes {
			if sc == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(s.dir(sc), id+".md")); err == nil {
				taken = true
			}
		}
		if !taken {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// Slug makes an id from a title: lower-case ASCII words joined by '-',
// at most 48 characters.
func Slug(title string) string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else if tr, ok := translit[r]; ok {
			cur.WriteString(tr)
		} else {
			flush()
		}
	}
	flush()
	var b strings.Builder
	for _, w := range words {
		if b.Len() > 0 && b.Len()+1+len(w) > 48 {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('-')
		}
		b.WriteString(w)
	}
	s := b.String()
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		s = "tip"
	}
	return s
}

// translit maps Ukrainian letters to ASCII for ids.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "h", 'ґ': "g", 'д': "d", 'е': "e", 'є': "ie", 'ж': "zh",
	'з': "z", 'и': "y", 'і': "i", 'ї': "i", 'й': "i", 'к': "k", 'л': "l", 'м': "m", 'н': "n",
	'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts",
	'ч': "ch", 'ш': "sh", 'щ': "shch", 'ь': "", 'ю': "iu", 'я': "ia", '\'': "", 'ʼ': "",
	'ы': "y", 'э': "e", 'ё': "e", 'ъ': "",
}

func (s *Store) write(t *Tip) error {
	return fsutil.WriteFileAtomic(t.Path, store.Compose(t.FM, t.Body), 0o644)
}

// SetVerified marks a tip verified today.
func (s *Store) SetVerified(t *Tip) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	t.FM.Set("status", Verified)
	t.FM.Set("verified", s.now().Format("2006-01-02"))
	t.load()
	return s.write(t)
}

// SetRefuted marks a tip refuted and appends the reason to its body.
func (s *Store) SetRefuted(t *Tip, why string) error {
	why = strings.TrimSpace(why)
	if why == "" {
		return fmt.Errorf("refuted needs a reason")
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	t.FM.Set("status", Refuted)
	t.FM.Del("verified")
	t.Body = strings.TrimRight(t.Body, "\n") + "\n\nRefuted (" + s.now().Format("2006-01-02") + "): " + why + "\n"
	t.load()
	return s.write(t)
}

// Supersede marks old as superseded by repl.
func (s *Store) Supersede(old, repl *Tip) error {
	if old.Path == repl.Path {
		return fmt.Errorf("a tip cannot supersede itself")
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	old.FM.Set("status", Superseded)
	old.FM.Set("superseded_by", repl.ID)
	old.load()
	return s.write(old)
}

// Move moves a tip to another scope, keeping its id.
func (s *Store) Move(t *Tip, scope string) (*Tip, error) {
	if t.Scope == scope {
		return nil, fmt.Errorf("tip %s is already in %s", t.ID, scope)
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	dst := filepath.Join(s.dir(scope), t.ID+".md")
	if _, err := os.Stat(dst); err == nil {
		return nil, fmt.Errorf("%s already has a tip %s", scope, t.ID)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := fsutil.Rename(t.Path, dst); err != nil {
		return nil, err
	}
	return Read(dst, scope)
}

// Sort orders tips by status (verified, active, others) then newest first.
func Sort(ts []*Tip) {
	rank := map[string]int{Verified: 0, Active: 1, Superseded: 2, Refuted: 3}
	sort.SliceStable(ts, func(i, j int) bool {
		if rank[ts[i].Status] != rank[ts[j].Status] {
			return rank[ts[i].Status] < rank[ts[j].Status]
		}
		return ts[i].Date().After(ts[j].Date())
	})
}

// Delete removes a tip file.
func (s *Store) Delete(t *Tip) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return os.Remove(t.Path)
}

// Scopes lists every directory in root that has tips (projects and global).
func (s *Store) Scopes() []string {
	ents, err := os.ReadDir(s.Root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			if st, err := os.Stat(s.dir(e.Name())); err == nil && st.IsDir() {
				out = append(out, e.Name())
			}
		}
	}
	return out
}
