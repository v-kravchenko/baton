package dashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/v-kravchenko/baton/internal/agent"
	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/gitinfo"
	"github.com/v-kravchenko/baton/internal/store"
	"github.com/v-kravchenko/baton/internal/tips"
)

type taskJSON struct {
	Task     string    `json:"task"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
	Branch   string    `json:"branch,omitempty"`
	Commit   string    `json:"commit,omitempty"`
	From     string    `json:"from,omitempty"`
	Archived bool      `json:"archived"`
	By       string    `json:"by,omitempty"`
}

func taskOf(h *store.Handoff) taskJSON {
	return taskJSON{Task: h.Task, Title: h.Title, Created: h.Created, Branch: h.Branch, Commit: h.Commit, From: h.From, Archived: h.Archived,
		By: h.FM.Get("by")}
}

type tipJSON struct {
	ID       string   `json:"id"`
	Scope    string   `json:"scope"`
	Title    string   `json:"title"`
	When     string   `json:"when,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Status   string   `json:"status"`
	Origin   string   `json:"origin,omitempty"`
	Source   []string `json:"source,omitempty"`
	Env      []string `json:"env,omitempty"`
	Cites    []string `json:"cites,omitempty"`
	Updated  string   `json:"updated,omitempty"`
	Body     string   `json:"body,omitempty"`
}

func tipOf(t *tips.Tip, body bool) tipJSON {
	j := tipJSON{ID: t.ID, Scope: t.Scope, Title: t.Title, When: t.When, Keywords: t.Keywords, Status: t.Status,
		Origin: t.Origin, Source: t.Source, Env: t.Env, Cites: t.Cites}
	// Tips carry only a date; the file time gives "5m ago" like handoffs.
	if fi, err := os.Stat(t.Path); err == nil {
		j.Updated = fi.ModTime().Format(time.RFC3339)
	}
	if body {
		j.Body = t.Body
	}
	return j
}

func (s *Server) paths() config.Paths {
	p, _ := config.LoadPaths(s.Dirs)
	return p
}

func (s *Server) projectParam(r *http.Request) (string, error) {
	p, err := store.TaskName(r.PathValue("p"))
	if err != nil || p == store.Global || !s.Store.Exists(p) {
		return "", errStatus(http.StatusNotFound, "no such project")
	}
	return p, nil
}

func (s *Server) taskParam(r *http.Request) (string, string, error) {
	p, err := s.projectParam(r)
	if err != nil {
		return "", "", err
	}
	t, err := store.TaskName(r.PathValue("t"))
	if err != nil {
		return "", "", errStatus(http.StatusNotFound, "no such task")
	}
	return p, t, nil
}

func (s *Server) apiProjects(w http.ResponseWriter, r *http.Request) (any, error) {
	paths := s.paths()
	type proj struct {
		Key       string     `json:"key"`
		Dir       string     `json:"dir,omitempty"`
		Repo      string     `json:"repo,omitempty"`
		Tasks     []taskJSON `json:"tasks"`
		Archived  int        `json:"archived"`
		Tips      int        `json:"tips"`
		Conflicts int        `json:"conflicts"`
		Updated   time.Time  `json:"updated"`
	}
	out := []proj{}
	for _, p := range s.Store.Projects() {
		active, _ := s.Store.Tasks(p)
		archived, _ := s.Store.Archived(p)
		ts, _ := s.Tips.List(p)
		pj := proj{Key: p, Dir: paths[p], Tasks: []taskJSON{}, Archived: len(archived), Tips: len(ts), Conflicts: len(s.Store.Conflicts(p))}
		if pj.Dir != "" {
			pj.Repo = gitinfo.RemoteURL(pj.Dir)
		}
		for _, h := range active {
			pj.Tasks = append(pj.Tasks, taskOf(h))
			if h.Created.After(pj.Updated) {
				pj.Updated = h.Created
			}
		}
		for _, h := range archived {
			if h.Created.After(pj.Updated) {
				pj.Updated = h.Created
			}
		}
		out = append(out, pj)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	global, _ := s.Tips.List(store.Global)
	return map[string]any{"projects": out, "global_tips": len(global), "root": s.cfg().Root, "home": s.Dirs.Home}, nil
}

func (s *Server) apiProject(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectParam(r)
	if err != nil {
		return nil, err
	}
	active, err := s.Store.Tasks(p)
	if err != nil {
		return nil, err
	}
	archived, err := s.Store.Archived(p)
	if err != nil {
		return nil, err
	}
	ts, err := s.Tips.List(p)
	if err != nil {
		return nil, err
	}
	tips.Sort(ts)
	out := map[string]any{"key": p, "dir": s.paths()[p], "tasks": []taskJSON{}, "archived": []taskJSON{}, "tips": []tipJSON{}, "conflicts": nonNil(s.Store.Conflicts(p))}
	var a, b []taskJSON
	for _, h := range active {
		a = append(a, taskOf(h))
	}
	for _, h := range archived {
		b = append(b, taskOf(h))
	}
	var tj []tipJSON
	for _, t := range ts {
		tj = append(tj, tipOf(t, false))
	}
	if a != nil {
		out["tasks"] = a
	}
	if b != nil {
		out["archived"] = b
	}
	if tj != nil {
		out["tips"] = tj
	}
	return out, nil
}

type pickupJSON struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
}

// pickups lists a copy button per agent.<name> line; none with built-in agents.
func (s *Server) pickups(p, t string) []pickupJSON {
	out := []pickupJSON{}
	if s.cfg().BuiltinAgents {
		return out
	}
	for _, a := range s.cfg().Agents {
		cmd := []string{"baton", "pickup", p, "@" + t}
		if a.Name != s.cfg().DefaultAgent {
			cmd = append(cmd, "--agent", a.Name)
		}
		out = append(out, pickupJSON{Agent: a.Name, Command: agent.Quote(cmd)})
	}
	return out
}

func (s *Server) apiTask(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	h, err := s.Store.Get(p, t)
	if err != nil {
		return nil, err
	}
	hist, err := s.Store.History(p, t)
	if err != nil {
		return nil, err
	}
	hj := []taskJSON{}
	for _, x := range hist {
		hj = append(hj, taskOf(x))
	}
	out := map[string]any{"project": p, "handoff": taskOf(h), "body": h.Body, "version": h.Version, "history": hj, "forks": nonNil(s.Store.Forks(p, t)), "pickup": s.pickups(p, t),
		"obsidian": s.cfg().ObsidianURL(h.Path)}
	if dir := s.paths()[p]; dir != "" {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			stale := gitinfo.Stale(dir, h.Branch, h.Commit)
			out["staleness"] = stale
			out["staleness_lines"] = stale.Lines()
		}
	}
	return out, nil
}

// version returns the current handoff (n = 0) or history entry n (1 = newest).
func (s *Server) version(p, t string, n int) (*store.Handoff, error) {
	if n == 0 {
		return s.Store.Get(p, t)
	}
	hist, err := s.Store.History(p, t)
	if err != nil {
		return nil, err
	}
	if n < 0 || n > len(hist) {
		return nil, errStatus(http.StatusNotFound, "no such history entry")
	}
	return hist[n-1], nil
}

func (s *Server) apiHistory(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		return nil, errStatus(http.StatusNotFound, "no such history entry")
	}
	h, err := s.version(p, t, n)
	if err != nil {
		return nil, err
	}
	return map[string]any{"handoff": taskOf(h), "body": h.Body}, nil
}

func (s *Server) apiDiff(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	from, err1 := strconv.Atoi(r.URL.Query().Get("from"))
	to := 0
	var err2 error
	if v := r.URL.Query().Get("to"); v != "" {
		to, err2 = strconv.Atoi(v)
	}
	if err1 != nil || err2 != nil {
		return nil, errStatus(http.StatusBadRequest, "from and to must be history numbers (0 = current)")
	}
	a, err := s.version(p, t, from)
	if err != nil {
		return nil, err
	}
	b, err := s.version(p, t, to)
	if err != nil {
		return nil, err
	}
	return map[string]any{"from": taskOf(a), "to": taskOf(b), "lines": Diff(a.Body, b.Body)}, nil
}

// save writes a handoff edited in the dashboard: the title follows the first
// heading, and branch/commit come from the project directory on this machine
// (or stay as they were without one).
func (s *Server) save(p, t string, in store.SaveInput) (any, error) {
	if strings.TrimSpace(in.Body) == "" {
		return nil, errStatus(http.StatusBadRequest, "the handoff is empty")
	}
	in.By = "dashboard"
	if in.Title == "" {
		in.Title = store.FirstHeading(in.Body)
	}
	var info gitinfo.Info
	ok := false
	if dir := s.paths()[p]; dir != "" {
		info, ok = gitinfo.Read(dir)
	}
	if ok {
		in.Branch, in.Commit = info.Branch, info.Commit
	} else if prev, err := s.Store.Get(p, t); err == nil {
		in.Branch, in.Commit = prev.Branch, prev.Commit
	}
	res, err := s.Store.Save(p, t, in)
	if err != nil {
		return nil, err
	}
	warns := store.SizeWarnings(in.Body)
	if len(res.Missing) > 0 {
		warns = append(warns, "missing sections: "+strings.Join(res.Missing, ", "))
	}
	return map[string]any{"handoff": taskOf(res.Handoff), "version": res.Handoff.Version, "warnings": nonNil(warns)}, nil
}

type saveJSON struct {
	Task    string `json:"task"`
	From    string `json:"from"`
	Body    string `json:"body"`
	Version string `json:"version"`
}

func decodeSave(r *http.Request) (saveJSON, error) {
	var in saveJSON
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return in, errStatus(http.StatusBadRequest, "bad request")
	}
	return in, nil
}

func (s *Server) apiTemplate(w http.ResponseWriter, r *http.Request) (any, error) {
	return map[string]any{"body": store.Template, "max_chars": store.MaxChars, "max_line": store.MaxLine}, nil
}

// apiCreate starts a task, a fork when "from" names its parent.
func (s *Server) apiCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	p, err := s.projectParam(r)
	if err != nil {
		return nil, err
	}
	in, err := decodeSave(r)
	if err != nil {
		return nil, err
	}
	t, err := store.TaskName(in.Task)
	if err != nil {
		return nil, err
	}
	from := ""
	if in.From != "" {
		if from, err = store.TaskName(in.From); err != nil {
			return nil, err
		}
		if _, err := s.Store.Get(p, from); err != nil {
			return nil, errStatus(http.StatusBadRequest, "no parent task @"+from)
		}
	}
	return s.save(p, t, store.SaveInput{Body: in.Body, From: from, New: true})
}

func (s *Server) apiSave(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	in, err := decodeSave(r)
	if err != nil {
		return nil, err
	}
	if in.Version == "" {
		return nil, errStatus(http.StatusBadRequest, "version is required")
	}
	return s.save(p, t, store.SaveInput{Body: in.Body, Expect: in.Version})
}

// apiRevert saves history entry n as the new handoff; the current one goes
// to history like on any save.
func (s *Server) apiRevert(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		return nil, errStatus(http.StatusNotFound, "no such history entry")
	}
	in, err := decodeSave(r)
	if err != nil {
		return nil, err
	}
	h, err := s.version(p, t, n)
	if err != nil {
		return nil, err
	}
	return s.save(p, t, store.SaveInput{Body: h.Body, Title: h.Title, Expect: in.Version})
}

func (s *Server) apiDone(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	h, err := s.Store.Done(p, t)
	if err != nil {
		return nil, err
	}
	return taskOf(h), nil
}

func (s *Server) apiRestore(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	h, err := s.Store.Restore(p, t)
	if err != nil {
		return nil, err
	}
	return taskOf(h), nil
}

func (s *Server) apiRename(w http.ResponseWriter, r *http.Request) (any, error) {
	p, t, err := s.taskParam(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, errStatus(http.StatusBadRequest, "bad request")
	}
	n, err := store.TaskName(in.Name)
	if err != nil {
		return nil, err
	}
	forks, err := s.Store.Rename(p, t, n)
	if err != nil {
		return nil, err
	}
	return map[string]any{"task": n, "forks_updated": forks}, nil
}

func (s *Server) tipScopes(r *http.Request) []string {
	if p := r.URL.Query().Get("project"); p != "" {
		if k, err := store.TaskName(p); err == nil {
			return []string{k, store.Global}
		}
		return []string{store.Global}
	}
	return s.Tips.Scopes()
}

func (s *Server) apiTips(w http.ResponseWriter, r *http.Request) (any, error) {
	all, err := s.Tips.List(s.tipScopes(r)...)
	if err != nil {
		return nil, err
	}
	out := []tipJSON{}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		tips.Sort(all)
		for _, t := range all {
			out = append(out, tipOf(t, false))
		}
		return out, nil
	}
	for _, res := range tips.Search(all, q, tips.Options{All: true, Limit: 50}) {
		out = append(out, tipOf(res.Tip, false))
	}
	return out, nil
}

func (s *Server) tipParam(r *http.Request) (*tips.Tip, error) {
	scope, err := store.TaskName(r.PathValue("scope"))
	if err != nil {
		return nil, errStatus(http.StatusNotFound, "no such tip")
	}
	t, err := s.Tips.Get(r.PathValue("id"), scope)
	if err != nil {
		return nil, errStatus(http.StatusNotFound, "no such tip")
	}
	return t, nil
}

func (s *Server) apiTip(w http.ResponseWriter, r *http.Request) (any, error) {
	t, err := s.tipParam(r)
	if err != nil {
		return nil, err
	}
	j := tipOf(t, true)
	return map[string]any{"tip": j, "version": t.Version, "superseded_by": t.FM.Get("superseded_by"), "verified": t.FM.Get("verified"),
		"obsidian": s.cfg().ObsidianURL(t.Path)}, nil
}

type tipEditJSON struct {
	Scope    string   `json:"scope"`
	Title    string   `json:"title"`
	When     string   `json:"when"`
	Keywords []string `json:"keywords"`
	Body     string   `json:"body"`
	Version  string   `json:"version"`
	Preview  bool     `json:"preview"`
	Why      string   `json:"why"`
}

func decodeTip(r *http.Request) (tipEditJSON, error) {
	var in tipEditJSON
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return in, errStatus(http.StatusBadRequest, "bad request")
	}
	return in, nil
}

// apiTipNew writes a tip with origin: user in a project or global scope.
func (s *Server) apiTipNew(w http.ResponseWriter, r *http.Request) (any, error) {
	in, err := decodeTip(r)
	if err != nil {
		return nil, err
	}
	scope, err := store.TaskName(in.Scope)
	if err != nil || (scope != store.Global && !s.Store.Exists(scope)) {
		return nil, errStatus(http.StatusBadRequest, "no such project")
	}
	nin := tips.NewInput{Scope: scope, Title: in.Title, When: in.When, Keywords: in.Keywords, Body: in.Body, Origin: "user"}
	lookup := []string{store.Global}
	if scope != store.Global {
		nin.Project = scope
		lookup = []string{scope, store.Global}
		if dir := s.paths()[scope]; dir != "" {
			if info, ok := gitinfo.Read(dir); ok {
				nin.Commit = info.Commit
			}
		}
	}
	// A body starting with "---" would be read as frontmatter: keep it text.
	if strings.HasPrefix(strings.TrimSpace(nin.Body), "---") {
		nin.Body = "\n" + nin.Body
	}
	t, warns, err := s.Tips.New(nin, lookup...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tip": tipOf(t, false), "warnings": nonNil(warns)}, nil
}

// apiTipEdit saves a tip, or with "preview" returns the diff the save would make.
func (s *Server) apiTipEdit(w http.ResponseWriter, r *http.Request) (any, error) {
	t, err := s.tipParam(r)
	if err != nil {
		return nil, err
	}
	in, err := decodeTip(r)
	if err != nil {
		return nil, err
	}
	if in.Version == "" {
		return nil, errStatus(http.StatusBadRequest, "version is required")
	}
	ein := tips.EditInput{Title: in.Title, When: in.When, Keywords: in.Keywords, Body: in.Body, Expect: in.Version}
	if in.Preview {
		if t.Version != in.Version {
			return nil, errStatus(http.StatusConflict, "the tip changed since it was opened")
		}
		old, err := os.ReadFile(t.Path)
		if err != nil {
			return nil, err
		}
		data, warns, err := tips.Edited(t, ein)
		if err != nil {
			return nil, err
		}
		return map[string]any{"lines": Diff(string(old), string(data)), "warnings": nonNil(warns)}, nil
	}
	nt, warns, err := s.Tips.Edit(t, ein)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tip": tipOf(nt, false), "warnings": nonNil(warns)}, nil
}

func (s *Server) apiTipVerified(w http.ResponseWriter, r *http.Request) (any, error) {
	t, err := s.tipParam(r)
	if err != nil {
		return nil, err
	}
	if err := s.Tips.SetVerified(t); err != nil {
		return nil, err
	}
	return tipOf(t, false), nil
}

func (s *Server) apiTipRefuted(w http.ResponseWriter, r *http.Request) (any, error) {
	t, err := s.tipParam(r)
	if err != nil {
		return nil, err
	}
	in, err := decodeTip(r)
	if err != nil {
		return nil, err
	}
	if err := s.Tips.SetRefuted(t, in.Why); err != nil {
		return nil, err
	}
	return tipOf(t, false), nil
}

func (s *Server) apiTipDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	t, err := s.tipParam(r)
	if err != nil {
		return nil, err
	}
	if err := s.Tips.Delete(t); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func (s *Server) apiSearch(w http.ResponseWriter, r *http.Request) (any, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	type hit struct {
		Project string   `json:"project"`
		Task    taskJSON `json:"task"`
		Snippet string   `json:"snippet,omitempty"`
	}
	out := map[string]any{"tasks": []hit{}, "tips": []tipJSON{}}
	if q == "" {
		return out, nil
	}
	words := tips.Tokens(q)
	var hits []hit
	for _, p := range s.Store.Projects() {
		for _, list := range [][]*store.Handoff{must(s.Store.Tasks(p)), must(s.Store.Archived(p))} {
			for _, h := range list {
				text := strings.ToLower(p + " " + h.Task + " " + h.Title + "\n" + h.Body)
				all := true
				for _, w := range words {
					all = all && strings.Contains(text, w)
				}
				if all {
					hits = append(hits, hit{Project: p, Task: taskOf(h), Snippet: snippet(h.Body, words)})
				}
			}
		}
	}
	if hits != nil {
		out["tasks"] = hits
	}
	allTips, _ := s.Tips.List(s.Tips.Scopes()...)
	var tj []tipJSON
	for _, res := range tips.Search(allTips, q, tips.Options{All: true, Limit: 30}) {
		tj = append(tj, tipOf(res.Tip, false))
	}
	if tj != nil {
		out["tips"] = tj
	}
	return out, nil
}

func must(hs []*store.Handoff, _ error) []*store.Handoff { return hs }

func snippet(body string, words []string) string {
	for _, l := range strings.Split(body, "\n") {
		ll := strings.ToLower(l)
		for _, w := range words {
			if strings.Contains(ll, w) {
				l = strings.TrimSpace(l)
				if r := []rune(l); len(r) > 160 {
					l = string(r[:160]) + "…"
				}
				return l
			}
		}
	}
	return ""
}

// nonNil keeps empty lists as [] in JSON instead of null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
