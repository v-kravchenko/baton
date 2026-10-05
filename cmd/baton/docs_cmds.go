package main

import (
	"fmt"
	"strings"

	"github.com/v-kravchenko/baton/internal/docs"
	"github.com/v-kravchenko/baton/internal/store"
)

func (app *App) docStore() *docs.Store {
	return &docs.Store{Root: app.Cfg.Root, LockDir: app.Dirs.State, Now: app.now}
}

type docRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Scope string `json:"scope"`
}

// docsForHandoff returns the docs the handoff links to with [[id]].
func (app *App) docsForHandoff(p string, h *store.Handoff) []docRef {
	var out []docRef
	for _, d := range app.docStore().Linked(h.Body, p, store.Global) {
		out = append(out, docRef{ID: d.ID, Title: d.Title, Scope: d.Scope})
	}
	return out
}

type docJSON struct {
	ID      string   `json:"id"`
	Scope   string   `json:"scope"`
	Title   string   `json:"title"`
	Created string   `json:"created"`
	Updated string   `json:"updated"`
	Version string   `json:"version"`
	Path    string   `json:"path"`
	UsedBy  []string `json:"used_by,omitempty"`
	Body    string   `json:"body,omitempty"`
}

func docToJSON(d *docs.Doc, body bool) docJSON {
	j := docJSON{ID: d.ID, Scope: d.Scope, Title: d.Title,
		Created: d.Created.Format("2006-01-02T15:04:05Z07:00"),
		Updated: d.Updated.Format("2006-01-02T15:04:05Z07:00"), Version: d.Version, Path: d.Path}
	if body {
		j.Body = d.Body
	}
	return j
}

func docLine(d *docs.Doc) string {
	return fmt.Sprintf("%s %s: %s", d.ID, d.Scope, d.Title)
}

func cmdDocs(app *App, a *args) error {
	sub, rest := "list", a.pos
	if len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	ds := app.docStore()
	scopes := app.tipScopes()
	get := func(usage string) (*docs.Doc, error) {
		if len(rest) != 1 {
			return nil, usageErr("expected docs %s ID", usage)
		}
		return ds.Get(rest[0], scopes...)
	}
	switch sub {
	case "list":
		if len(rest) > 0 {
			return usageErr("unexpected argument %q", rest[0])
		}
		all, err := ds.List(scopes...)
		if err != nil {
			return err
		}
		out := []docJSON{}
		for _, d := range all {
			out = append(out, docToJSON(d, false))
		}
		if a.bools["json"] {
			return app.writeJSON(out)
		}
		if len(out) == 0 {
			app.printf("no docs\n")
			return nil
		}
		for _, d := range all {
			app.printf("%s\n", docLine(d))
		}
		return nil

	case "show":
		d, err := get("show")
		if err != nil {
			return err
		}
		var used []string
		if d.Scope != store.Global {
			used = docs.Backlinks(app.store(), d.Scope, d.ID)
		}
		if a.bools["json"] {
			j := docToJSON(d, true)
			j.UsedBy = used
			return app.writeJSON(j)
		}
		app.printf("%s\n", docLine(d))
		app.printf("updated %s\n", d.Updated.Format("2006-01-02 15:04 -07:00"))
		if len(used) > 0 {
			app.printf("used by: @%s\n", strings.Join(used, ", @"))
		}
		app.printf("\n%s", d.Body)
		return nil

	case "new":
		if len(rest) > 0 {
			return usageErr("unexpected argument %q", rest[0])
		}
		body, err := app.readStdin("docs new", "doc.md")
		if err != nil {
			return err
		}
		in := docs.NewInput{Body: body, ID: a.vals["id"], Title: a.vals["title"]}
		if a.bools["global"] {
			in.Scope = store.Global
		} else if p, err := app.project(); err != nil {
			return fmt.Errorf("%v (use --global for a global doc)", err)
		} else {
			in.Scope = p
		}
		d, warns, err := ds.New(in)
		if err != nil {
			return err
		}
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"doc": docToJSON(d, false), "warnings": warns})
		}
		for _, w := range warns {
			app.warnf("%s", w)
		}
		app.printf("saved doc %s (%s): %s\nlink it from a handoff as [[%s]]\n", d.ID, d.Scope, d.Path, d.ID)
		return nil

	case "edit":
		d, err := get("edit")
		if err != nil {
			return err
		}
		text, err := app.readStdin("docs edit "+d.ID, "doc.md")
		if err != nil {
			return err
		}
		fm, body, err := store.Split([]byte(text))
		if err != nil {
			return err
		}
		pick := func(flag, key, cur string) string {
			if v := a.vals[flag]; v != "" {
				return v
			}
			if v := fm.Get(key); v != "" {
				return v
			}
			return cur
		}
		e, warns, err := ds.Edit(d, docs.EditInput{Title: pick("title", "title", d.Title), Body: body, Expect: a.vals["expect"]})
		if err != nil {
			return err
		}
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"doc": docToJSON(e, false), "warnings": warns})
		}
		for _, w := range warns {
			app.warnf("%s", w)
		}
		app.printf("saved doc %s (%s)\n", e.ID, e.Scope)
		return nil

	case "delete":
		d, err := get("delete")
		if err != nil {
			return err
		}
		if err := ds.Delete(d); err != nil {
			return err
		}
		app.printf("deleted doc %s (%s)\n", d.ID, d.Scope)
		return nil
	}
	return usageErr("unknown docs subcommand %q", sub)
}
