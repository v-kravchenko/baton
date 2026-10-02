package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/gitinfo"
	"github.com/v-kravchenko/baton/internal/store"
)

// Show states.
const (
	stateOK         = "OK"
	stateChoose     = "CHOOSE TASK"
	stateNoTask     = "NO TASK"
	stateArchived   = "ARCHIVED"
	stateNoHandoff  = "NO HANDOFF"
	defaultTaskName = "main"
)

func (app *App) now() time.Time {
	if v := os.Getenv("BATON_NOW"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Now()
}

func (app *App) store() *store.Store {
	return &store.Store{Root: app.Cfg.Root, LockDir: app.Dirs.State, Keep: app.Cfg.Keep, Now: app.now}
}

func (app *App) project() (string, error) { return store.ProjectKey(app.Cwd) }

// remember records cwd as the directory of project key on this machine.
func (app *App) remember(key string) {
	if err := config.Remember(app.Dirs, key, app.Cwd); err != nil {
		app.warnf("cannot update paths: %v", err)
	}
}

func (app *App) conflictWarnings(st *store.Store, p string) []string {
	var out []string
	for _, c := range st.Conflicts(p) {
		out = append(out, "sync conflict: "+filepath.Join(app.Cfg.Root, filepath.FromSlash(c))+" (merge by hand, then delete it)")
	}
	return out
}

type handoffJSON struct {
	Task     string    `json:"task"`
	Title    string    `json:"title"`
	Created  time.Time `json:"created"`
	Branch   string    `json:"branch,omitempty"`
	Commit   string    `json:"commit,omitempty"`
	From     string    `json:"from,omitempty"`
	Archived bool      `json:"archived"`
	Path     string    `json:"path"`
	Obsidian string    `json:"obsidian,omitempty"`
	Body     string    `json:"body,omitempty"`
}

func toJSON(h *store.Handoff, withBody bool) handoffJSON {
	j := handoffJSON{Task: h.Task, Title: h.Title, Created: h.Created, Branch: h.Branch, Commit: h.Commit, From: h.From, Archived: h.Archived, Path: h.Path}
	if withBody {
		j.Body = h.Body
	}
	return j
}

func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 0:
		return "in the future"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func taskArg(s string) (string, error) { return store.TaskName(s) }

// resolveSaveTask picks the task for `save` without @task.
func resolveSaveTask(st *store.Store, p string) (string, error) {
	ts, err := st.Tasks(p)
	if err != nil {
		return "", err
	}
	switch len(ts) {
	case 0:
		return defaultTaskName, nil
	case 1:
		return ts[0].Task, nil
	}
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = "@" + t.Task
	}
	return "", fmt.Errorf("project has several tasks (%s); pass one: baton save @task", strings.Join(names, ", "))
}

func cmdSave(app *App, a *args) error {
	task, rest := splitTask(a.pos)
	if len(rest) > 0 {
		return usageErr("unexpected argument %q", rest[0])
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	st := app.store()
	if task == "" {
		if task, err = resolveSaveTask(st, p); err != nil {
			return err
		}
	} else if task, err = taskArg(task); err != nil {
		return err
	}
	from := a.vals["from"]
	if from != "" {
		if from, err = taskArg(from); err != nil {
			return err
		}
		if _, err := st.Get(p, from); err != nil {
			app.warnf("parent task @%s does not exist in project %s", from, p)
		}
	}
	body, err := io.ReadAll(app.Stdin)
	if err != nil {
		return err
	}
	in := store.SaveInput{Body: string(body), Title: a.vals["title"], From: from}
	if info, ok := gitinfo.Read(app.Cwd); ok {
		in.Branch, in.Commit = info.Branch, info.Commit
	}
	res, err := st.Save(p, task, in)
	if err != nil {
		return err
	}
	app.remember(p)
	warns := app.conflictWarnings(st, p)
	if len(res.Missing) > 0 {
		warns = append(warns, "missing sections: "+strings.Join(res.Missing, ", ")+" (see baton template)")
	}
	warns = append(warns, store.SizeWarnings(in.Body)...)
	if a.bools["json"] {
		return app.writeJSON(map[string]any{
			"project": p, "handoff": toJSON(res.Handoff, false), "rotated": res.Rotated,
			"pruned": res.Pruned, "restored": res.Restored, "missing": res.Missing, "warnings": warns,
		})
	}
	for _, w := range warns {
		app.warnf("%s", w)
	}
	app.printf("saved %s/@%s: %s\n", p, task, res.Handoff.Path)
	if res.Restored {
		app.printf("task was archived; it is active again\n")
	}
	if res.Rotated != "" {
		app.printf("previous handoff: %s\n", res.Rotated)
	}
	return nil
}

type showResult struct {
	State     string             `json:"state"`
	Project   string             `json:"project"`
	Message   string             `json:"message,omitempty"`
	Handoff   *handoffJSON       `json:"handoff,omitempty"`
	Tasks     []handoffJSON      `json:"tasks"`
	Archived  []handoffJSON      `json:"archived"`
	Forks     []string           `json:"forks,omitempty"`
	Staleness *gitinfo.Staleness `json:"staleness,omitempty"`
	Tips      []tipRef           `json:"tips,omitempty"`
	Warnings  []string           `json:"warnings,omitempty"`
	handoff   *store.Handoff
}

type tipRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Scope string `json:"scope"`
}

func cmdShow(app *App, a *args) error {
	task, rest := splitTask(a.pos)
	if len(rest) > 1 {
		return usageErr("too many arguments")
	}
	if len(rest) == 1 {
		if task != "" {
			return usageErr("pass either @task or FILE")
		}
		return showFile(app, a, rest[0])
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	st := app.store()
	if task != "" {
		if task, err = taskArg(task); err != nil {
			return err
		}
	}
	if st.Exists(p) {
		app.remember(p)
	}
	r, err := buildShow(app, st, p, task)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return app.writeJSON(r)
	}
	printShow(app, r)
	return nil
}

func buildShow(app *App, st *store.Store, p, task string) (*showResult, error) {
	r := &showResult{Project: p, Tasks: []handoffJSON{}, Archived: []handoffJSON{}}
	active, err := st.Tasks(p)
	if err != nil {
		return nil, err
	}
	archived, err := st.Archived(p)
	if err != nil {
		return nil, err
	}
	for _, h := range active {
		r.Tasks = append(r.Tasks, toJSON(h, false))
	}
	for _, h := range archived {
		r.Archived = append(r.Archived, toJSON(h, false))
	}
	r.Warnings = app.conflictWarnings(st, p)

	var h *store.Handoff
	switch {
	case task == "" && len(active) == 0:
		r.State = stateNoHandoff
		r.Message = fmt.Sprintf("project %s has no active handoff", p)
		return r, nil
	case task == "" && len(active) > 1:
		r.State = stateChoose
		r.Message = fmt.Sprintf("project %s has %d active tasks; ask the user which one, then run baton show @task", p, len(active))
		return r, nil
	case task == "":
		h = active[0]
	default:
		h, err = st.Get(p, task)
		if errors.Is(err, fs.ErrNotExist) {
			r.State = stateNoTask
			r.Message = fmt.Sprintf("no task @%s in project %s; ask the user to pick an existing task or start a new one", task, p)
			return r, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Archived {
			r.State = stateArchived
			r.Message = fmt.Sprintf("@%s is archived (done); ask the user whether to restore it: baton restore %s", task, task)
			j := toJSON(h, false)
			r.Handoff = &j
			return r, nil
		}
	}
	r.State = stateOK
	r.handoff = h
	j := toJSON(h, true)
	j.Obsidian = app.Cfg.ObsidianURL(h.Path)
	r.Handoff = &j
	r.Forks = st.Forks(p, h.Task)
	s := gitinfo.Stale(app.Cwd, h.Branch, h.Commit)
	r.Staleness = &s
	r.Tips = app.tipsForHandoff(p, h)
	return r, nil
}

func printShow(app *App, r *showResult) {
	now := app.now()
	if r.State != stateOK {
		app.printf("STATE: %s\n%s\n", r.State, r.Message)
		if r.Handoff != nil {
			app.printf("\n@%s: %s (saved %s)\n", r.Handoff.Task, r.Handoff.Title, ago(r.Handoff.Created, now))
		}
		if len(r.Tasks) > 0 {
			app.printf("\nactive tasks:\n")
			for _, t := range r.Tasks {
				app.printf("  @%s: %s (%s)\n", t.Task, t.Title, ago(t.Created, now))
			}
		}
		if len(r.Archived) > 0 && r.State != stateArchived {
			app.printf("\narchived tasks:\n")
			for _, t := range r.Archived {
				app.printf("  @%s: %s (%s)\n", t.Task, t.Title, ago(t.Created, now))
			}
		}
		printWarnings(app, r.Warnings)
		return
	}
	h := r.handoff
	app.printf("@%s: %s\n", h.Task, h.Title)
	app.printf("project: %s · saved %s (%s)%s\n", r.Project, h.Created.Format("2006-01-02 15:04 -07:00"), ago(h.Created, now), gitSuffix(h))
	var rel []string
	if h.From != "" {
		rel = append(rel, "from: @"+h.From)
	}
	if len(r.Forks) > 0 {
		rel = append(rel, "forks: @"+strings.Join(r.Forks, ", @"))
	}
	var others []string
	for _, t := range r.Tasks {
		if t.Task != h.Task {
			others = append(others, "@"+t.Task)
		}
	}
	if len(others) > 0 {
		rel = append(rel, "other tasks: "+strings.Join(others, ", "))
	}
	if len(rel) > 0 {
		app.printf("%s\n", strings.Join(rel, " · "))
	}
	if lines := r.Staleness.Lines(); len(lines) > 0 {
		app.printf("\nstaleness:\n")
		for _, l := range lines {
			app.printf("  %s\n", l)
		}
	}
	if len(r.Tips) > 0 {
		app.printf("\ntips that may apply (full text: baton tips show ID):\n")
		for _, t := range r.Tips {
			app.printf("  %s: %s\n", t.ID, t.Title)
		}
	}
	printWarnings(app, r.Warnings)
	app.printf("\n---\n\n%s", h.Body)
	if !strings.HasSuffix(h.Body, "\n") {
		app.printf("\n")
	}
}

func printWarnings(app *App, warns []string) {
	if len(warns) == 0 {
		return
	}
	app.printf("\nwarnings:\n")
	for _, w := range warns {
		app.printf("  %s\n", w)
	}
}

func gitSuffix(h *store.Handoff) string {
	switch {
	case h.Branch != "" && h.Commit != "":
		return " · " + h.Branch + " @ " + h.Commit
	case h.Commit != "":
		return " · @ " + h.Commit
	case h.Branch != "":
		return " · " + h.Branch
	}
	return ""
}

func showFile(app *App, a *args, file string) error {
	h, err := store.ReadFile(file)
	if err != nil {
		return err
	}
	s := gitinfo.Stale(app.Cwd, h.Branch, h.Commit)
	if a.bools["json"] {
		j := toJSON(h, true)
		return app.writeJSON(&showResult{State: stateOK, Handoff: &j, Staleness: &s, Tasks: []handoffJSON{}, Archived: []handoffJSON{}})
	}
	r := &showResult{State: stateOK, Project: "(file)", handoff: h, Staleness: &s}
	printShow(app, r)
	return nil
}

func cmdTasks(app *App, a *args) error {
	if len(a.pos) > 0 {
		return usageErr("unexpected argument %q", a.pos[0])
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	st := app.store()
	active, err := st.Tasks(p)
	if err != nil {
		return err
	}
	archived, err := st.Archived(p)
	if err != nil {
		return err
	}
	warns := app.conflictWarnings(st, p)
	if a.bools["json"] {
		r := map[string]any{"project": p, "tasks": []handoffJSON{}, "archived": []handoffJSON{}, "warnings": warns}
		var ts, as []handoffJSON
		for _, h := range active {
			ts = append(ts, toJSON(h, false))
		}
		for _, h := range archived {
			as = append(as, toJSON(h, false))
		}
		if ts != nil {
			r["tasks"] = ts
		}
		if as != nil {
			r["archived"] = as
		}
		return app.writeJSON(r)
	}
	now := app.now()
	if len(active) == 0 {
		app.printf("project %s: no active tasks\n", p)
	} else {
		app.printf("project %s:\n", p)
		for _, h := range active {
			extra := ""
			if h.From != "" {
				extra = " (fork of @" + h.From + ")"
			}
			app.printf("  @%s: %s, %s%s%s\n", h.Task, h.Title, ago(h.Created, now), gitSuffix(h), extra)
		}
	}
	if len(archived) > 0 {
		names := make([]string, len(archived))
		for i, h := range archived {
			names[i] = "@" + h.Task
		}
		app.printf("archived: %s\n", strings.Join(names, ", "))
	}
	for _, w := range warns {
		app.printf("warning: %s\n", w)
	}
	return nil
}

func oneTask(a *args) (string, error) {
	if len(a.pos) != 1 {
		return "", usageErr("expected one task name")
	}
	return taskArg(a.pos[0])
}

func cmdDone(app *App, a *args) error {
	t, err := oneTask(a)
	if err != nil {
		return err
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	h, err := app.store().Done(p, t)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return app.writeJSON(toJSON(h, false))
	}
	app.printf("archived %s/@%s\n", p, t)
	return nil
}

func cmdRestore(app *App, a *args) error {
	t, err := oneTask(a)
	if err != nil {
		return err
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	h, err := app.store().Restore(p, t)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return app.writeJSON(toJSON(h, false))
	}
	app.printf("restored %s/@%s\n", p, t)
	return nil
}

func cmdRename(app *App, a *args) error {
	if len(a.pos) != 2 {
		return usageErr("expected OLD NEW")
	}
	oldT, err := taskArg(a.pos[0])
	if err != nil {
		return err
	}
	newT, err := taskArg(a.pos[1])
	if err != nil {
		return err
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	forks, err := app.store().Rename(p, oldT, newT)
	if err != nil {
		return err
	}
	if a.bools["json"] {
		return app.writeJSON(map[string]any{"project": p, "old": oldT, "new": newT, "forks_updated": forks})
	}
	app.printf("renamed %s/@%s → @%s\n", p, oldT, newT)
	if len(forks) > 0 {
		app.printf("updated from: in @%s\n", strings.Join(forks, ", @"))
	}
	return nil
}

func cmdHistory(app *App, a *args) error {
	if len(a.pos) < 1 || len(a.pos) > 2 {
		return usageErr("expected TASK [N]")
	}
	t, err := taskArg(a.pos[0])
	if err != nil {
		return err
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	hs, err := app.store().History(p, t)
	if err != nil {
		return err
	}
	if len(a.pos) == 2 {
		n, err := strconv.Atoi(a.pos[1])
		if err != nil || n < 1 || n > len(hs) {
			return fmt.Errorf("no history entry %s for @%s (%d entries)", a.pos[1], t, len(hs))
		}
		h := hs[n-1]
		if a.bools["json"] {
			return app.writeJSON(toJSON(h, true))
		}
		app.printf("@%s history #%d: %s\nsaved %s%s\n\n---\n\n%s", t, n, h.Title, h.Created.Format("2006-01-02 15:04 -07:00"), gitSuffix(h), h.Body)
		return nil
	}
	if a.bools["json"] {
		out := []handoffJSON{}
		for _, h := range hs {
			out = append(out, toJSON(h, false))
		}
		return app.writeJSON(out)
	}
	if len(hs) == 0 {
		app.printf("@%s has no history\n", t)
		return nil
	}
	now := app.now()
	for i, h := range hs {
		app.printf("%d  %s  %s (%s)%s\n", i+1, h.Created.Format("2006-01-02 15:04"), h.Title, ago(h.Created, now), gitSuffix(h))
	}
	return nil
}

func cmdStale(app *App, a *args) error {
	if len(a.pos) != 1 {
		return usageErr("expected FILE or @task")
	}
	var h *store.Handoff
	var err error
	if strings.HasPrefix(a.pos[0], "@") {
		var t, p string
		if t, err = taskArg(a.pos[0]); err != nil {
			return err
		}
		if p, err = app.project(); err != nil {
			return err
		}
		h, err = app.store().Get(p, t)
	} else {
		h, err = store.ReadFile(a.pos[0])
	}
	if err != nil {
		return err
	}
	s := gitinfo.Stale(app.Cwd, h.Branch, h.Commit)
	if a.bools["json"] {
		return app.writeJSON(s)
	}
	lines := s.Lines()
	if len(lines) == 0 {
		lines = []string{"no git data in the handoff and no repository here"}
	}
	for _, l := range lines {
		app.printf("%s\n", l)
	}
	return nil
}

func cmdTemplate(app *App, a *args) error {
	if a.bools["json"] {
		return app.writeJSON(map[string]any{"sections": store.Sections, "template": store.Template})
	}
	app.printf("%s", store.Template)
	return nil
}
