package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/store"
	"github.com/v-kravchenko/baton/internal/tips"
)

type env struct {
	t   *testing.T
	srv *Server
	h   http.Handler
}

func newEnv(t *testing.T, auth bool) *env {
	root, state := t.TempDir(), t.TempDir()
	now := func() time.Time { return time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC) }
	st := &store.Store{Root: root, LockDir: state, Keep: 5, Now: now}
	ts := &tips.Store{Root: root, LockDir: state, Now: now}
	cfg := config.Defaults("/home/u")
	cfg.Root, cfg.Auth = root, auth
	cfg.Agents = config.DefaultAgents
	cfg.DefaultAgent = "claude"
	cfg.PublicURL = "https://baton.example.com"
	dirs := config.Dirs{Home: "/home/u", Config: t.TempDir(), State: state}
	s := New(cfg, dirs, st, ts, "test")
	st.Save("api", "auth", store.SaveInput{Body: "## Goal\nfirst\n", Title: "Auth"})
	st.Save("api", "auth", store.SaveInput{Body: "## Goal\nsecond\n"})
	ts.New(tips.NewInput{Scope: "api", Title: "Flock on NFS", Body: "Tip: x\nVerify: y\n"})
	return &env{t: t, srv: s, h: s.Handler()}
}

type reqOpt func(*http.Request)

func local(r *http.Request)  { r.RemoteAddr = "127.0.0.1:50000" }
func remote(r *http.Request) { r.RemoteAddr = "192.168.1.20:50000"; r.Host = "192.168.1.5:8765" }
func header(k, v string) reqOpt {
	return func(r *http.Request) { r.Header.Set(k, v) }
}
func cookie(c *http.Cookie) reqOpt { return func(r *http.Request) { r.AddCookie(c) } }

func (e *env) do(method, path, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Host = "127.0.0.1:8765"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return m
}

func TestLocalAccessAndCSRF(t *testing.T) {
	e := newEnv(t, false)
	w := e.do("GET", "/api/session", "", local)
	s := decode(t, w)
	csrf, _ := s["csrf"].(string)
	if w.Code != 200 || csrf == "" || s["local"] != true {
		t.Fatalf("session = %d %v", w.Code, s)
	}
	if w := e.do("GET", "/api/projects", "", local); w.Code != 200 || !strings.Contains(w.Body.String(), `"key":"api"`) {
		t.Errorf("projects = %d %s", w.Code, w.Body)
	}
	w = e.do("GET", "/api/projects/api/tasks/auth", "", local)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "baton pickup api @auth --agent opencode") {
		t.Errorf("task = %d %s", w.Code, w.Body)
	}
	w = e.do("GET", "/api/projects/api/tasks/auth/diff?from=1", "", local)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `{"op":"+","text":"second"}`) {
		t.Errorf("diff = %d %s", w.Code, w.Body)
	}
	// Unsafe method without / with wrong token.
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", local); w.Code != 403 {
		t.Errorf("done without csrf = %d", w.Code)
	}
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", local, header(csrfHeader, "x")); w.Code != 403 {
		t.Errorf("done with bad csrf = %d", w.Code)
	}
	// Cross-origin POST is refused even with the token.
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", local, header(csrfHeader, csrf), header("Origin", "http://evil.test")); w.Code != 403 {
		t.Errorf("cross-origin done = %d", w.Code)
	}
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", local, header(csrfHeader, csrf), header("Origin", "http://127.0.0.1:8765")); w.Code != 200 {
		t.Errorf("done = %d %s", w.Code, w.Body)
	}
	if w := e.do("POST", "/api/projects/api/tasks/auth/rename", `{"name":"login"}`, local, header(csrfHeader, csrf)); w.Code != 200 {
		t.Errorf("rename = %d %s", w.Code, w.Body)
	}
	if w := e.do("POST", "/api/projects/api/tasks/login/restore", "", local, header(csrfHeader, csrf)); w.Code != 200 {
		t.Errorf("restore = %d %s", w.Code, w.Body)
	}
	if w := e.do("GET", "/api/search?q=second", "", local); !strings.Contains(w.Body.String(), `"task":"login"`) {
		t.Errorf("search = %s", w.Body)
	}
	if w := e.do("DELETE", "/api/tips/api/flock-on-nfs", "", local, header(csrfHeader, csrf)); w.Code != 200 {
		t.Errorf("delete tip = %d %s", w.Code, w.Body)
	}
	if w := e.do("GET", "/api/projects/..%2fetc", "", local); w.Code != 404 {
		t.Errorf("traversal = %d", w.Code)
	}
}

func TestHostCheck(t *testing.T) {
	e := newEnv(t, false)
	for host, want := range map[string]int{
		"evil.example:8765":   421,
		"localhost:8765":      200,
		"[::1]:8765":          200,
		"10.0.0.7:8765":       200,
		"baton.example.com":   403, // allowed host, but not trusted as local
		"baton.example.com.x": 421,
	} {
		w := e.do("GET", "/api/session", "", local, func(r *http.Request) { r.Host = host })
		if w.Code != want {
			t.Errorf("host %s = %d, want %d", host, w.Code, want)
		}
	}
}

func TestProxyIsNotLocal(t *testing.T) {
	e := newEnv(t, false)
	if w := e.do("GET", "/api/projects", "", local, header("X-Forwarded-For", "1.2.3.4")); w.Code != 403 {
		t.Errorf("forwarded = %d", w.Code)
	}
	if w := e.do("GET", "/api/projects", "", local, func(r *http.Request) { r.Host = "baton.example.com" }); w.Code != 403 {
		t.Errorf("public host via loopback = %d", w.Code)
	}
}

func TestRemoteAuth(t *testing.T) {
	e := newEnv(t, false)
	if w := e.do("GET", "/api/projects", "", remote); w.Code != 403 {
		t.Errorf("remote with auth off = %d", w.Code)
	}
	if w := e.do("GET", "/", "", remote); w.Code != 403 {
		t.Errorf("remote page with auth off = %d", w.Code)
	}

	e = newEnv(t, true)
	// Auth on but no password: still no access.
	if w := e.do("GET", "/api/projects", "", remote); w.Code != 403 {
		t.Errorf("auth on, no password = %d", w.Code)
	}
	if err := e.srv.Auth.SetPassword("short"); err == nil {
		t.Error("short password accepted")
	}
	if err := e.srv.Auth.SetPassword("correct horse"); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/projects", "", remote); w.Code != 401 {
		t.Errorf("not logged in = %d", w.Code)
	}
	if w := e.do("GET", "/", "", remote); w.Code != 303 || w.Header().Get("Location") != "/login" {
		t.Errorf("index redirect = %d %v", w.Code, w.Header())
	}
	if w := e.do("POST", "/api/login", `{"password":"nope"}`, remote); w.Code != 401 {
		t.Errorf("wrong password = %d", w.Code)
	}
	w := e.do("POST", "/api/login", `{"password":"correct horse"}`, remote)
	if w.Code != 200 {
		t.Fatalf("login = %d %s", w.Code, w.Body)
	}
	c := w.Result().Cookies()[0]
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || !c.Secure {
		t.Errorf("cookie flags = %+v", c)
	}
	s := decode(t, e.do("GET", "/api/session", "", remote, cookie(c)))
	csrf, _ := s["csrf"].(string)
	if s["authenticated"] != true || csrf == "" || csrf == e.srv.localCSRF {
		t.Errorf("session = %v", s)
	}
	if w := e.do("GET", "/api/projects", "", remote, cookie(c)); w.Code != 200 {
		t.Errorf("logged in = %d", w.Code)
	}
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", remote, cookie(c), header(csrfHeader, e.srv.localCSRF)); w.Code != 403 {
		t.Errorf("local csrf accepted for remote session = %d", w.Code)
	}
	if w := e.do("POST", "/api/projects/api/tasks/auth/done", "", remote, cookie(c), header(csrfHeader, csrf)); w.Code != 200 {
		t.Errorf("remote done = %d %s", w.Code, w.Body)
	}
	if err := e.srv.Auth.LogoutAll(); err != nil {
		t.Fatal(err)
	}
	if w := e.do("GET", "/api/projects", "", remote, cookie(c)); w.Code != 401 {
		t.Errorf("after logout-all = %d", w.Code)
	}
}

func TestLockout(t *testing.T) {
	e := newEnv(t, true)
	e.srv.Auth.SetPassword("correct horse")
	for i := 0; i < lockAfter; i++ {
		e.do("POST", "/api/login", `{"password":"bad"}`, remote)
	}
	w := e.do("POST", "/api/login", `{"password":"correct horse"}`, remote)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Errorf("locked login = %d %v", w.Code, w.Header())
	}
}

func TestSessionExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := &Auth{StateDir: t.TempDir(), Idle: time.Hour, Max: 3 * time.Hour, Now: func() time.Time { return now }}
	tok, _, err := a.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		now = now.Add(50 * time.Minute)
		if _, ok := a.Session(tok); !ok {
			t.Fatalf("session expired early at step %d", i)
		}
	}
	now = now.Add(61 * time.Minute)
	if _, ok := a.Session(tok); ok {
		t.Error("idle session still valid")
	}
	tok, _, _ = a.NewSession()
	for i := 0; i < 4; i++ {
		now = now.Add(50 * time.Minute)
		a.Session(tok)
	}
	if _, ok := a.Session(tok); ok {
		t.Error("session valid past max age")
	}
}

func TestDiff(t *testing.T) {
	got := Diff("a\nb\nc\n", "a\nc\nd\n")
	want := []DiffLine{{" ", "a"}, {"-", "b"}, {" ", "c"}, {"+", "d"}}
	if len(got) != len(want) {
		t.Fatalf("diff = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestStaticAndPages(t *testing.T) {
	e := newEnv(t, false)
	if w := e.do("GET", "/static/app.js", "", local); w.Code != 200 || !strings.Contains(w.Body.String(), "renderMarkdown") {
		t.Errorf("app.js = %d", w.Code)
	}
	w := e.do("GET", "/", "", local)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Errorf("index = %d %v", w.Code, w.Header())
	}
	if w := e.do("GET", "/login", "", local); w.Code != 303 {
		t.Errorf("login page for local = %d", w.Code)
	}
}

func TestEmptyListsAreNotNull(t *testing.T) {
	e := newEnv(t, false)
	for _, path := range []string{
		"/api/projects",
		"/api/projects/api",
		"/api/projects/api/tasks/auth",
		"/api/projects/api/tasks/auth/diff?from=1&to=1",
		"/api/search?q=nothing-matches",
	} {
		w := e.do("GET", path, "", local)
		if w.Code != 200 || strings.Contains(w.Body.String(), "null") {
			t.Errorf("%s = %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestPickupButtonsOnlyForConfiguredAgents(t *testing.T) {
	e := newEnv(t, false)
	n := func() int {
		w := e.do("GET", "/api/projects/api/tasks/auth", "", local)
		return len(decode(t, w)["pickup"].([]any))
	}
	if got := n(); got != 2 {
		t.Errorf("configured agents: %d buttons", got)
	}
	e.srv.Cfg.BuiltinAgents = true
	if got := n(); got != 0 {
		t.Errorf("built-in agents: %d buttons", got)
	}
}

func TestPublicURLReload(t *testing.T) {
	e := newEnv(t, true)
	host := func(h string) reqOpt { return func(r *http.Request) { r.Host = h } }
	if w := e.do("GET", "/login", "", local, host("baton.example.com")); w.Code == http.StatusMisdirectedRequest {
		t.Fatalf("old host = %d", w.Code)
	}
	cfg := e.srv.Cfg
	cfg.PublicURL = "https://new.example.com"
	e.srv.LoadConfig = func() (config.Config, error) { return cfg, nil }
	e.srv.checked = time.Time{}
	if w := e.do("GET", "/login", "", local, host("new.example.com")); w.Code == http.StatusMisdirectedRequest {
		t.Fatalf("new host = %d %s", w.Code, w.Body)
	}
	if w := e.do("GET", "/login", "", local, host("baton.example.com")); w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("dropped host = %d", w.Code)
	}
}

func TestObsidianLinks(t *testing.T) {
	e := newEnv(t, false)
	if w := e.do("GET", "/api/projects/api/tasks/auth", "", local); strings.Contains(w.Body.String(), `"obsidian":"obsidian:`) {
		t.Errorf("link without obsidian.vault: %s", w.Body)
	}
	e.srv.Cfg.ObsidianVault = filepath.Dir(e.srv.Cfg.Root)
	folder := filepath.Base(e.srv.Cfg.Root)
	for path, file := range map[string]string{
		"/api/projects/api/tasks/auth": folder + "%2Fapi%2Ftasks%2Fauth.md",
		"/api/tips/api/flock-on-nfs":   folder + "%2Fapi%2Ftips%2Fflock-on-nfs.md",
	} {
		m := decode(t, e.do("GET", path, "", local))
		if got, want := m["obsidian"], "obsidian://open?vault="+url.QueryEscape(filepath.Base(e.srv.Cfg.ObsidianVault))+"&file="+file; got != want {
			t.Errorf("%s: obsidian = %v, want %s", path, got, want)
		}
	}
}

func TestWriteAPI(t *testing.T) {
	e := newEnv(t, false)
	csrf := decode(t, e.do("GET", "/api/session", "", local))["csrf"].(string)
	tok := header(csrfHeader, csrf)
	d := decode(t, e.do("GET", "/api/projects/api/tasks/auth", "", local))
	ver := d["version"].(string)

	w := e.do("PUT", "/api/projects/api/tasks/auth", `{"body":"# Login\n## Goal\nthird\n","version":"`+ver+`"}`, local, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Login"`) || !strings.Contains(w.Body.String(), `"by":"dashboard"`) {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("PUT", "/api/projects/api/tasks/auth", `{"body":"x","version":"`+ver+`"}`, local, tok); w.Code != 409 {
		t.Errorf("stale save: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("PUT", "/api/projects/api/tasks/auth", `{"body":"x"}`, local, tok); w.Code != 400 {
		t.Errorf("save without version: %d", w.Code)
	}

	w = e.do("POST", "/api/projects/api/tasks", `{"task":"@Auth-UI","from":"auth","body":"## Goal\nfork\n"}`, local, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"from":"auth"`) || !strings.Contains(w.Body.String(), `"task":"auth-ui"`) {
		t.Fatalf("fork: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/projects/api/tasks", `{"task":"auth-ui","body":"x"}`, local, tok); w.Code != 409 {
		t.Errorf("create over existing: %d", w.Code)
	}
	if w := e.do("POST", "/api/projects/api/tasks", `{"task":"z","from":"nope","body":"x"}`, local, tok); w.Code != 400 {
		t.Errorf("missing parent: %d", w.Code)
	}

	// History 1 is now "second"; restoring it makes it current.
	ver = decode(t, e.do("GET", "/api/projects/api/tasks/auth", "", local))["version"].(string)
	if w := e.do("POST", "/api/projects/api/tasks/auth/history/1/restore", `{"version":"`+ver+`"}`, local, tok); w.Code != 200 {
		t.Fatalf("revert: %d %s", w.Code, w.Body.String())
	}
	if d := decode(t, e.do("GET", "/api/projects/api/tasks/auth", "", local)); !strings.Contains(d["body"].(string), "second") {
		t.Errorf("revert body: %q", d["body"])
	}
	if w := e.do("GET", "/api/template", "", local); w.Code != 200 || !strings.Contains(w.Body.String(), "max_chars") {
		t.Errorf("template: %d", w.Code)
	}
}

func TestTipWriteAPI(t *testing.T) {
	e := newEnv(t, false)
	csrf := decode(t, e.do("GET", "/api/session", "", local))["csrf"].(string)
	tok := header(csrfHeader, csrf)
	w := e.do("POST", "/api/tips", `{"scope":"api","title":"Mine","keywords":["a"],"body":"Tip: t\nWhy: w\nVerify: v\n"}`, local, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"origin":"user"`) || !strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Fatalf("new: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/tips", `{"scope":"nope","title":"x","body":"y"}`, local, tok); w.Code != 400 {
		t.Errorf("new in a missing project: %d", w.Code)
	}
	ver := decode(t, e.do("GET", "/api/tips/api/mine", "", local))["version"].(string)
	edit := `{"title":"Mine 2","keywords":["a","b"],"body":"Tip: t2\nWhy: w\nVerify: v\n","version":"` + ver + `"`
	w = e.do("PUT", "/api/tips/api/mine", edit+`,"preview":true}`, local, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"op":"+","text":"Tip: t2"`) {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	if d := decode(t, e.do("GET", "/api/tips/api/mine", "", local)); d["version"] != ver {
		t.Error("preview wrote the file")
	}
	if w := e.do("PUT", "/api/tips/api/mine", edit+`}`, local, tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"Mine 2"`) {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("PUT", "/api/tips/api/mine", edit+`}`, local, tok); w.Code != 409 {
		t.Errorf("stale edit: %d", w.Code)
	}
	if w := e.do("POST", "/api/tips/api/mine/verified", "", local, tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"verified"`) {
		t.Errorf("verified: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/tips/api/mine/refuted", `{"why":""}`, local, tok); w.Code != 400 {
		t.Errorf("refuted without a reason: %d", w.Code)
	}
	if w := e.do("POST", "/api/tips/api/mine/refuted", `{"why":"wrong"}`, local, tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"refuted"`) {
		t.Errorf("refuted: %d %s", w.Code, w.Body.String())
	}
}

func TestCheckAPI(t *testing.T) {
	e := newEnv(t, false)
	csrf := decode(t, e.do("GET", "/api/session", "", local))["csrf"].(string)
	tok := header(csrfHeader, csrf)
	e.srv.Store.Save("api", "list", store.SaveInput{Body: "## Next steps\n1. [ ] go\n"})
	d := decode(t, e.do("GET", "/api/projects/api/tasks/list", "", local))
	n := 0
	for i, l := range strings.Split(d["body"].(string), "\n") {
		if strings.HasPrefix(l, "1. [ ]") {
			n = i
		}
	}
	body := `{"line":` + strconv.Itoa(n) + `,"checked":true,"version":"` + d["version"].(string) + `"}`
	if w := e.do("POST", "/api/projects/api/tasks/list/check", body, local, tok); w.Code != 200 || !strings.Contains(w.Body.String(), "[x] go") {
		t.Fatalf("check: %d %s", w.Code, w.Body.String())
	}
	if w := e.do("POST", "/api/projects/api/tasks/list/check", body, local, tok); w.Code != 409 {
		t.Errorf("stale check: %d", w.Code)
	}
}

func TestSlugAPI(t *testing.T) {
	e := newEnv(t, false)
	for title, want := range map[string]string{"Auth": "auth-2", "Termux TLS!": "termux-tls", "Перевірка": "perevirka", "!!!": "", "con": "con-task"} {
		got := decode(t, e.do("GET", "/api/projects/api/slug?title="+url.QueryEscape(title), "", local))["task"]
		if got != want {
			t.Errorf("%q = %v, want %q", title, got, want)
		}
	}
}

func TestPWAAssets(t *testing.T) {
	e := newEnv(t, true)
	if w := e.do("GET", "/sw.js", "", remote); w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("sw.js: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	w := e.do("GET", "/manifest.webmanifest", "", remote)
	var m map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &m) != nil || m["start_url"] != "/" {
		t.Errorf("manifest: %d %s", w.Code, w.Body.String())
	}
	for _, icon := range m["icons"].([]any) {
		src := icon.(map[string]any)["src"].(string)
		if w := e.do("GET", src, "", remote); w.Code != 200 {
			t.Errorf("%s: %d", src, w.Code)
		}
	}
}
