package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitAndRender(t *testing.T) {
	in := "---\r\ntitle: \"Refresh \\\"token\\\" flow\"\r\ncreated: 2026-10-01T10:15:00+03:00   # c\r\nkeywords: [go, \"a, b\", 'it''s']\r\nempty:\r\n---\r\n\r\n# Body\r\n"
	fm, body, err := Split([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := fm.Get("title"); got != `Refresh "token" flow` {
		t.Errorf("title = %q", got)
	}
	if got := fm.Get("created"); got != "2026-10-01T10:15:00+03:00" {
		t.Errorf("created = %q", got)
	}
	if got := fm.GetList("keywords"); !reflect.DeepEqual(got, []string{"go", "a, b", "it's"}) {
		t.Errorf("keywords = %q", got)
	}
	if body != "# Body\n" {
		t.Errorf("body = %q", body)
	}
	out := string(Compose(fm, body))
	fm2, body2, err := Split([]byte(out))
	if err != nil || !reflect.DeepEqual(fm, fm2) || body2 != "# Body\n" {
		t.Errorf("round trip: %q\n%+v %q %v", out, fm2, body2, err)
	}
}

func TestSplitNoFrontmatter(t *testing.T) {
	fm, body, err := Split([]byte("hello\n---\n"))
	if err != nil || len(fm.Fields) != 0 || body != "hello\n---\n" {
		t.Errorf("%+v %q %v", fm, body, err)
	}
	if _, _, err := Split([]byte("---\ntitle: x\n")); err == nil {
		t.Error("unterminated frontmatter accepted")
	}
	fm, body, err = Split([]byte("---\n---\nb"))
	if err != nil || len(fm.Fields) != 0 || body != "b" {
		t.Errorf("empty fm: %+v %q %v", fm, body, err)
	}
}

func TestQuoting(t *testing.T) {
	fm := &Frontmatter{}
	fm.Set("a", "x: y")
	fm.Set("b", "plain")
	fm.SetList("c", []string{"one", "t,wo"})
	fm.Set("d", "")
	// YAML indicators at the start would make Obsidian reject the block.
	fm.SetList("e", []string{"{width=", "-Dscreen", "&a", "x-y"})
	fm.Set("f", "|x")
	want := "---\na: \"x: y\"\nb: plain\nc: [one, \"t,wo\"]\n" +
		"e: [\"{width=\", \"-Dscreen\", \"&a\", x-y]\nf: \"|x\"\n---\n"
	if got := fm.Render(); got != want {
		t.Errorf("render = %q", got)
	}
}

// Obsidian's Properties editor writes block lists, nested values and fields
// baton does not know; they must parse and survive a rewrite unchanged.
func TestObsidianFields(t *testing.T) {
	head := "title: \"T\"\n" +
		"tags:\n  - work\n  - \"a: b\"\n" +
		"aliases:\n- one\n\n- two\n" +
		"cssclasses: [wide]\n" +
		"empty:\n" +
		"note: |\n  line 1\n  line 2\n" +
		"cover:\n  url: x.png\n  # c\n  size: 2\n" +
		"refs:\n  - id: 1\n" +
		"long: first\n  second\n" +
		"from: parent\n"
	fm, body, err := Split([]byte("---\n" + head + "---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if body != "body\n" {
		t.Errorf("body = %q", body)
	}
	for key, want := range map[string][]string{
		"tags": {"work", "a: b"}, "aliases": {"one", "two"}, "cssclasses": {"wide"},
		"cover": nil, "refs": nil, "empty": nil,
	} {
		if got := fm.GetList(key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]string{
		"note": "line 1\nline 2", "long": "first second", "from": "parent", "url": "", "size": "",
	} {
		if got := fm.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	fm.Set("from", "other")
	fm.Set("created", "2026-10-01T10:00:00Z")
	want := strings.Replace(head, "from: parent", "from: other", 1) + "created: 2026-10-01T10:00:00Z\n"
	if got := fm.Render(); got != "---\n"+want+"---\n" {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}
	c := fm.Clone()
	c.SetList("tags", []string{"x"})
	if got := fm.GetList("tags"); len(got) != 2 {
		t.Errorf("clone shares fields: %q", got)
	}
	if got := c.Render(); !strings.Contains(got, "\ntags: [x]\naliases:\n") {
		t.Errorf("replaced list not re-rendered in place:\n%s", got)
	}
	for _, bad := range []string{"  title: x\n", "- a\n", "title\n"} {
		if _, _, err := Split([]byte("---\n" + bad + "---\n")); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
