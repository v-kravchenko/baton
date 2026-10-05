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
}

// docsForHandoff returns the docs attached to the task of the handoff.
func (app *App) docsForHandoff(p string, h *store.Handoff) []docRef {
	ds, _ := app.docStore().List(p, h.Task)
	var out []docRef
	for _, d := range ds {
		out = append(out, docRef{ID: d.ID, Title: d.Title})
	}
	return out
}

type docJSON struct {
	ID      string `json:"id"`
	Task    string `json:"task"`
	Title   string `json:"title"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Body    string `json:"body,omitempty"`
}

func docToJSON(d *docs.Doc, body bool) docJSON {
	j := docJSON{ID: d.ID, Task: d.Task, Title: d.Title,
		Created: d.Created.Format("2006-01-02T15:04:05Z07:00"),
		Updated: d.Updated.Format("2006-01-02T15:04:05Z07:00"), Version: d.Version, Path: d.Path}
	if body {
		j.Body = d.Body
	}
	return j
}

func docLine(d *docs.Doc) string {
	return fmt.Sprintf("@%s %s: %s", d.Task, d.ID, d.Title)
}

func cmdDocs(app *App, a *args) error {
	sub, rest := "list", a.pos
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "@") {
		sub, rest = rest[0], rest[1:]
	}
	p, err := app.project()
	if err != nil {
		return err
	}
	task := ""
	if len(rest) > 0 && strings.HasPrefix(rest[0], "@") {
		if task, err = store.TaskName(rest[0]); err != nil {
			return err
		}
		rest = rest[1:]
	}
	ds := app.docStore()
	needTask := func(usage string) error {
		if task == "" {
			return usageErr("expected docs %s @task", usage)
		}
		return nil
	}
	get := func(usage string) (*docs.Doc, error) {
		if err := needTask(usage + " ID"); err != nil {
			return nil, err
		}
		if len(rest) != 1 {
			return nil, usageErr("expected docs %s @task ID", usage)
		}
		return ds.Get(p, task, rest[0])
	}
	switch sub {
	case "list":
		if len(rest) > 0 {
			return usageErr("unexpected argument %q", rest[0])
		}
		all, err := ds.List(p, task)
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
		if a.bools["json"] {
			return app.writeJSON(docToJSON(d, true))
		}
		app.printf("%s\n", docLine(d))
		app.printf("updated %s\n", d.Updated.Format("2006-01-02 15:04 -07:00"))
		app.printf("\n%s", d.Body)
		return nil

	case "new":
		if err := needTask("new"); err != nil {
			return err
		}
		if len(rest) > 0 {
			return usageErr("unexpected argument %q", rest[0])
		}
		body, err := app.readStdin("docs new", "doc.md")
		if err != nil {
			return err
		}
		d, warns, err := ds.New(docs.NewInput{Project: p, Task: task, Body: body, ID: a.vals["id"], Title: a.vals["title"]})
		if err != nil {
			return err
		}
		if a.bools["json"] {
			return app.writeJSON(map[string]any{"doc": docToJSON(d, false), "warnings": warns})
		}
		for _, w := range warns {
			app.warnf("%s", w)
		}
		app.printf("saved doc %s of @%s: %s\n", d.ID, d.Task, d.Path)
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
		app.printf("saved doc %s of @%s\n", e.ID, e.Task)
		return nil

	case "delete":
		d, err := get("delete")
		if err != nil {
			return err
		}
		if err := ds.Delete(d); err != nil {
			return err
		}
		app.printf("deleted doc %s of @%s\n", d.ID, d.Task)
		return nil
	}
	return usageErr("unknown docs subcommand %q", sub)
}
