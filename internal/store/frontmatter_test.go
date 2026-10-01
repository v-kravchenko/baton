package store

import (
	"reflect"
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
	want := "---\na: \"x: y\"\nb: plain\nc: [one, \"t,wo\"]\n---\n"
	if got := fm.Render(); got != want {
		t.Errorf("render = %q", got)
	}
}
