package agent

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := map[string][]string{
		`claude "/pickup {task}"`:                   {"claude", "/pickup {task}"},
		`C:\tools\x.exe --dir 'a b' c"d e"f`:        {`C:\tools\x.exe`, "--dir", "a b", "cd ef"},
		`  opencode   --prompt   "/pickup {task}" `: {"opencode", "--prompt", "/pickup {task}"},
		`x ""`: {"x", ""},
	}
	for in, want := range cases {
		got, err := Split(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q) = %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`x "y`, "   "} {
		if _, err := Split(bad); err == nil {
			t.Errorf("Split(%q) accepted", bad)
		}
	}
}

func TestExpand(t *testing.T) {
	v := Vars{Project: "p", Dir: "/d", Root: "/r"}
	got, _ := Build(`claude "/pickup {task}" {task} --dir={dir} {project};rm`, v)
	want := []string{"claude", "/pickup", "--dir=/d", "p;rm"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	v.Task = "@api"
	got, _ = Build(`claude "/pickup {task}"`, v)
	if !reflect.DeepEqual(got, []string{"claude", "/pickup @api"}) {
		t.Errorf("got %q", got)
	}
	v.Prompt = "start at {task} step 2"
	got, _ = Build(`claude "/pickup {task}"`, v)
	if !reflect.DeepEqual(got, []string{"claude", "/pickup @api start at {task} step 2"}) {
		t.Errorf("prompt after task: got %q", got)
	}
	got, _ = Build(`x {task} --msg={prompt}`, v)
	if !reflect.DeepEqual(got, []string{"x", "@api", "--msg=start at {task} step 2"}) {
		t.Errorf("{prompt}: got %q", got)
	}
	if _, err := Build(`x {project}`, v); err == nil {
		t.Error("prompt without a place accepted")
	}
}

func TestQuote(t *testing.T) {
	if got := quotePOSIX("/pickup @api") + " " + quotePOSIX("it's"); got != `'/pickup @api' 'it'\''s'` {
		t.Errorf("posix: %s", got)
	}
	if got := quoteWindows(`C:\work\api`) + " " + quoteWindows("/pickup @api"); got != `C:\work\api "/pickup @api"` {
		t.Errorf("windows: %s", got)
	}
}
