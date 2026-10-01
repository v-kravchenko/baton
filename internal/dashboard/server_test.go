package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
