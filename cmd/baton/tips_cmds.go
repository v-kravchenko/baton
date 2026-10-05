package main

import (
	"fmt"
	"strings"

	"github.com/v-kravchenko/baton/internal/gitinfo"
	"github.com/v-kravchenko/baton/internal/store"
	"github.com/v-kravchenko/baton/internal/tips"
)

func (app *App) tipStore() *tips.Store {
	return &tips.Store{Root: app.Cfg.Root, LockDir: app.Dirs.State, Now: app.now}
}

// tipScopes returns the lookup order: current project (if any), then global.
func (app *App) tipScopes() []string {
	if p, err := app.project(); err == nil {
		return []string{p, store.Global}
	}
	return []string{store.Global}
}

// tipsForHandoff suggests tips matching the task name, title and Context
// (Gotchas in handoffs written before 0.1.8).
func (app *App) tipsForHandoff(p string, h *store.Handoff) []tipRef {
	all, err := app.tipStore().List(p, store.Global)
	if err != nil || len(all) == 0 {
		return nil
	}
	q := strings.Join([]string{strings.ReplaceAll(h.Task, "-", " "), h.Title, store.Section(h.Body, "Context"), store.Section(h.Body, "Gotchas")}, " ")
	var out []tipRef
	for _, r := range tips.Search(all, q, tips.Options{NoBody: true, MinScore: 4, Limit: 5}) {
		out = append(out, tipRef{ID: r.Tip.ID, Title: r.Tip.Title, Scope: r.Tip.Scope})
	}
	return out
}

type tipJSON struct {
	ID       string   `json:"id"`
	Scope    string   `json:"scope"`
	Title    string   `json:"title"`
	When     string   `json:"when,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Cites    []string `json:"cites,omitempty"`
	Origin   string   `json:"origin,omitempty"`
	Source   []string `json:"source,omitempty"`
	Status   string   `json:"status"`
	Env      []string `json:"env,omitempty"`
	Path     string   `json:"path"`
	Body     string   `json:"body,omitempty"`
	Score    float64  `json:"score,omitempty"`
}

func tipToJSON(t *tips.Tip, body bool) tipJSON {
	j := tipJSON{ID: t.ID, Scope: t.Scope, Title: t.Title, When: t.When, Keywords: t.Keywords, Cites: t.Cites,
		Origin: t.Origin, Source: t.Source, Status: t.Status, Env: t.Env, Path: t.Path}
	if body {
		j.Body = t.Body
	}
	return j
}

func tipLine(t *tips.Tip) string {
	status := ""
	if t.Status != tips.Active {
		status = " [" + t.Status + "]"
	}
	when := ""
	if t.When != "" {
		when = " (when: " + t.When + ")"
	}
	return fmt.Sprintf("%s%s %s: %s%s", t.ID, status, t.Scope, t.Title, when)
}

func cmdTips(app *App, a *args) error {
	if len(a.pos) == 0 {
		return usageErr("expected a tips subcommand")
	}
	sub, rest := a.pos[0], a.pos[1:]
	ts := app.tipStore()
	scopes := app.tipScopes()
	switch sub {
	case "search":
		q := strings.Join(rest, " ")
		if q == "" && a.bools["error"] {
			b, err := app.readStdin("tips search --error", "error.txt")
			if err != nil {
				return err
			}
			q = b
		}
		if strings.TrimSpace(q) == "" {
			return usageErr("expected search words")
		}
		all, err := ts.List(scopes...)
		if err != nil {
			return err
		}
		res := tips.Search(all, q, tips.Options{Error: a.bools["error"], All: a.bools["all"], Limit: 10})
		if a.bools["json"] {
			out := []tipJSON{}
			for _, r := range res {
				j := tipToJSON(r.Tip, false)
				j.Score = r.Score
				out = append(out, j)
			}
			return app.writeJSON(out)
		}
		if len(res) == 0 {
			app.printf("no matching tips\n")
			return nil
		}
		for _, r := range res {
			app.printf("%s\n", tipLine(r.Tip))
		}
		app.printf("\nfull text: baton tips show ID; check its Verify step before relying on it\n")
		return nil

	case "list":
		all, err := ts.List(scopes...)
		if err != nil {
			return err
		}
		tips.Sort(all)
		if a.bools["json"] {
			out := []tipJSON{}
			for _, t := range all {
				if a.bools["all"] || t.Live() {
					out = append(out, tipToJSON(t, false))
				}
			}
			return app.writeJSON(out)
		}
		n := 0
		for _, t := range all {
			if a.bools["all"] || t.Live() {
				app.printf("%s\n", tipLine(t))
				n++
			}
		}
		if n == 0 {
			app.printf("no tips\n")
		}
		return nil

	case "show":
		if len(rest) != 1 {
			return usageErr("expected tips show ID")
		}
		t, err := ts.Get(rest[0], scopes...)
		if err != nil {
			return err
		}
		if a.bools["json"] {
			return app.writeJSON(tipToJSON(t, true))
		}
		app.printf("%s\n", tipLine(t))
		if len(t.Keywords) > 0 {
			app.printf("keywords: %s\n", strings.Join(t.Keywords, ", "))
		}
		if len(t.Cites) > 0 {
			app.printf("cites: %s\n", strings.Join(t.Cites, ", "))
		}
		if len(t.Env) > 0 {
			app.printf("env: %s\n", strings.Join(t.Env, ", "))
		}
		if len(t.Source) > 0 {
			app.printf("source: %s · origin: %s\n", strings.Join(t.Source, " "), t.Origin)
		}
		if by := t.FM.Get("superseded_by"); by != "" {
			app.printf("superseded by: %s\n", by)
		}
		app.printf("\n%s", t.Body)
		return nil

	case "new":
		if len(rest) > 0 {
			return usageErr("unexpected argument %q", rest[0])
		}
		body, err := app.readStdin("tips new", "tip.md")
		if err != nil {
			return err
		}
		in := tips.NewInput{
			Body: body, Title: a.vals["title"], When: a.vals["when"], Origin: a.vals["origin"],
			Keywords: splitList(a.vals["keywords"]), Cites: splitList(a.vals["cites"]), Env: splitList(a.vals["env"]),
		}
		p, perr := app.project()
		if perr == nil {
			in.Project = p
			if info, ok := gitinfo.Read(app.Cwd); ok {
				in.Commit = info.Commit
			}
		}
		if a.bools["global"] {
			in.Scope = store.Global
		} else if perr != nil {
			return fmt.Errorf("%v (use --global for a global tip)", perr)
		} else {
			in.Scope = p
		}
		existing, _ := ts.List(scopes...)
		t, warns, err := ts.New(in, scopes...)
		if err != nil {
			return err
		}
		var dups []string
		for _, r := range tips.Search(existing, t.Title+" "+strings.Join(t.Keywords, " "), tips.Options{NoBody: true, MinScore: 6, Limit: 3}) {
			dups = append(dups, r.Tip.ID)
		}
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"tip": tipToJSON(t, false), "warnings": warns, "similar": dups})
		}
		for _, w := range warns {
			app.warnf("%s", w)
		}
		if len(dups) > 0 {
			app.warnf("similar tips exist: %s (baton tips supersede OLD %s if this replaces one)", strings.Join(dups, ", "), t.ID)
		}
		app.printf("saved tip %s (%s): %s\n", t.ID, t.Scope, t.Path)
		return nil

	case "verified":
		if len(rest) != 1 {
			return usageErr("expected tips verified ID")
		}
		t, err := ts.Get(rest[0], scopes...)
		if err != nil {
			return err
		}
		if err := ts.SetVerified(t); err != nil {
			return err
		}
		app.printf("tip %s: verified\n", t.ID)
		return nil

	case "refuted":
		if len(rest) < 2 {
			return usageErr("expected tips refuted ID WHY")
		}
		t, err := ts.Get(rest[0], scopes...)
		if err != nil {
			return err
		}
		if err := ts.SetRefuted(t, strings.Join(rest[1:], " ")); err != nil {
			return err
		}
		app.printf("tip %s: refuted\n", t.ID)
		return nil

	case "supersede":
		if len(rest) != 2 {
			return usageErr("expected tips supersede OLD NEW")
		}
		old, err := ts.Get(rest[0], scopes...)
		if err != nil {
			return err
		}
		repl, err := ts.Get(rest[1], scopes...)
		if err != nil {
			return err
		}
		if err := ts.Supersede(old, repl); err != nil {
			return err
		}
		app.printf("tip %s: superseded by %s\n", old.ID, repl.ID)
		return nil

	case "move":
		if len(rest) != 2 || (rest[1] != "global" && rest[1] != "project") {
			return usageErr("expected tips move ID global|project")
		}
		t, err := ts.Get(rest[0], scopes...)
		if err != nil {
			return err
		}
		scope := store.Global
		if rest[1] == "project" {
			p, err := app.project()
			if err != nil {
				return err
			}
			scope = p
		}
		m, err := ts.Move(t, scope)
		if err != nil {
			return err
		}
		app.printf("tip %s moved to %s: %s\n", m.ID, m.Scope, m.Path)
		return nil
	}
	return usageErr("unknown tips subcommand %q", sub)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
