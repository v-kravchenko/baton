// Package dashboard serves the web UI and its JSON API over the store and
// tips. Access is all or nothing: trusted loopback clients and logged-in
// sessions get everything, everyone else gets nothing.
package dashboard

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/v-kravchenko/baton/internal/config"
	"github.com/v-kravchenko/baton/internal/store"
	"github.com/v-kravchenko/baton/internal/tips"
)

//go:embed web
var webFS embed.FS

const (
	cookieName = "baton_session"
	csrfHeader = "X-Baton-Token"
	maxBody    = 64 << 10
)

// Server is the dashboard.
type Server struct {
	Cfg     config.Config
	Dirs    config.Dirs
	Store   *store.Store
	Tips    *tips.Store
	Auth    *Auth
	Version string

	// LoadConfig, if set, is polled so config edits (auth on/off, agents,
	// public_url) apply without a restart. Root, host and port stay fixed.
	LoadConfig func() (config.Config, error)

	localCSRF string
	hostnames map[string]bool
	mu        sync.Mutex
	checked   time.Time
}

const reloadEvery = 2 * time.Second

func (s *Server) cfg() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LoadConfig != nil && time.Since(s.checked) > reloadEvery {
		s.checked = time.Now()
		if c, err := s.LoadConfig(); err == nil {
			c.Root, c.Host, c.Port = s.Cfg.Root, s.Cfg.Host, s.Cfg.Port
			s.Cfg = c
			s.Auth.setLimits(c.AuthIdle, c.AuthMax)
			if u, err := url.Parse(c.PublicURL); err == nil && u.Hostname() != "" {
				s.hostnames[strings.ToLower(u.Hostname())] = true
			}
		}
	}
	return s.Cfg
}

// New builds a server from config.
func New(cfg config.Config, dirs config.Dirs, st *store.Store, ts *tips.Store, version string) *Server {
	s := &Server{Cfg: cfg, Dirs: dirs, Store: st, Tips: ts, Version: version}
	s.Auth = &Auth{StateDir: dirs.State, Idle: cfg.AuthIdle, Max: cfg.AuthMax, Now: st.Now}
	s.localCSRF = randomToken()
	s.hostnames = map[string]bool{"localhost": true}
	if h, err := os.Hostname(); err == nil && h != "" {
		h = strings.ToLower(h)
		s.hostnames[h] = true
		s.hostnames[strings.TrimSuffix(h, ".local")+".local"] = true
	}
	if cfg.Host != "" && net.ParseIP(cfg.Host) == nil {
		s.hostnames[strings.ToLower(cfg.Host)] = true
	}
	if u, err := url.Parse(cfg.PublicURL); err == nil && u.Hostname() != "" {
		s.hostnames[strings.ToLower(u.Hostname())] = true
	}
	return s
}

// Handler returns the HTTP handler with all checks applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", s.page("index.html", true))
	mux.HandleFunc("GET /login", s.page("login.html", false))
	mux.HandleFunc("GET /api/session", s.apiSession)
	mux.HandleFunc("POST /api/login", s.apiLogin)
	mux.HandleFunc("POST /api/logout", s.apiLogout)

	api := func(pattern string, h func(http.ResponseWriter, *http.Request) (any, error)) {
		mux.HandleFunc(pattern, s.protected(h))
	}
	api("GET /api/projects", s.apiProjects)
	api("GET /api/projects/{p}", s.apiProject)
	api("GET /api/projects/{p}/tasks/{t}", s.apiTask)
	api("GET /api/projects/{p}/tasks/{t}/history/{n}", s.apiHistory)
	api("GET /api/projects/{p}/tasks/{t}/diff", s.apiDiff)
	api("POST /api/projects/{p}/tasks/{t}/done", s.apiDone)
	api("POST /api/projects/{p}/tasks/{t}/restore", s.apiRestore)
	api("POST /api/projects/{p}/tasks/{t}/rename", s.apiRename)
	api("GET /api/tips", s.apiTips)
	api("GET /api/tips/{scope}/{id}", s.apiTip)
	api("DELETE /api/tips/{scope}/{id}", s.apiTipDelete)
	api("GET /api/search", s.apiSearch)
	return s.guard(mux)
}

// guard applies Host (DNS rebinding) and Origin checks and security headers.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if !s.hostAllowed(r.Host) {
			http.Error(w, "baton: unknown Host header (add the name to dashboard.public_url)", http.StatusMisdirectedRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.originAllowed(r) {
			http.Error(w, "baton: cross-origin request refused", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.ToLower(strings.Trim(name, "[]"))
	if name == "" {
		return false
	}
	// IP literals cannot be rebound by DNS.
	if net.ParseIP(name) != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hostnames[name]
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Browsers always send Origin on cross-site POST/DELETE; fall back to
		// Fetch Metadata where present.
		site := r.Header.Get("Sec-Fetch-Site")
		return site == "" || site == "same-origin" || site == "none"
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if p, err := url.Parse(s.cfg().PublicURL); err == nil && p.Host != "" && strings.EqualFold(p.Host, u.Host) && p.Scheme == u.Scheme {
		return true
	}
	return false
}

// trustedLocal: a direct loopback client, not a request relayed by a proxy.
func (s *Server) trustedLocal(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "Forwarded", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	if u, err := url.Parse(s.cfg().PublicURL); err == nil && u.Hostname() != "" {
		name, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			name = r.Host
		}
		if strings.EqualFold(name, u.Hostname()) && net.ParseIP(u.Hostname()) == nil {
			return false
		}
	}
	return true
}

type access int

const (
	accessNone  access = iota // remote and auth off
	accessLogin               // remote, auth on, not logged in
	accessFull
)

func (s *Server) access(r *http.Request) (access, string) {
	if s.trustedLocal(r) {
		return accessFull, s.localCSRF
	}
	if !s.cfg().Auth || !s.Auth.HasPassword() {
		return accessNone, ""
	}
	if c, err := r.Cookie(cookieName); err == nil {
		if csrf, ok := s.Auth.Session(c.Value); ok {
			return accessFull, csrf
		}
	}
	return accessLogin, ""
}

const noAccessMsg = "baton: remote access is disabled; on the dashboard machine run `baton auth password` and `baton auth on`"

func (s *Server) page(name string, app bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc, _ := s.access(r)
		switch {
		case acc == accessNone:
			http.Error(w, noAccessMsg, http.StatusForbidden)
			return
		case app && acc == accessLogin:
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		case !app && acc == accessFull:
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		data, err := webFS.ReadFile("web/" + name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	}
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func errStatus(code int, msg string) error { return httpError{code, msg} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	var he httpError
	if errors.As(err, &he) {
		writeJSON(w, he.code, map[string]string{"error": he.msg})
		return
	}
	if errors.Is(err, fs.ErrNotExist) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

// protected requires full access, and a CSRF token for unsafe methods.
func (s *Server) protected(h func(http.ResponseWriter, *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc, csrf := s.access(r)
		switch acc {
		case accessNone:
			writeErr(w, errStatus(http.StatusForbidden, noAccessMsg))
			return
		case accessLogin:
			writeErr(w, errStatus(http.StatusUnauthorized, "login required"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get(csrfHeader)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(csrf)) != 1 {
				writeErr(w, errStatus(http.StatusForbidden, "missing or invalid CSRF token"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		v, err := h(w, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(s.cfg().PublicURL, "https://")
}

func (s *Server) apiSession(w http.ResponseWriter, r *http.Request) {
	acc, csrf := s.access(r)
	out := map[string]any{"version": s.Version, "authenticated": acc == accessFull, "login_required": acc == accessLogin}
	if acc == accessNone {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": noAccessMsg})
		return
	}
	if acc == accessFull {
		out["csrf"] = csrf
		out["local"] = s.trustedLocal(r)
		var agents []string
		for _, a := range s.cfg().Agents {
			agents = append(agents, a.Name)
		}
		out["agents"] = agents
		out["default_agent"] = s.cfg().DefaultAgent
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	acc, _ := s.access(r)
	if acc == accessNone {
		writeErr(w, errStatus(http.StatusForbidden, noAccessMsg))
		return
	}
	client := clientKey(r)
	if d := s.Auth.Locked(client); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeErr(w, errStatus(http.StatusTooManyRequests, "too many failed attempts; try again in "+d.Round(time.Second).String()))
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeErr(w, errStatus(http.StatusBadRequest, "bad request"))
		return
	}
	ok, err := s.Auth.CheckPassword(in.Password)
	if err != nil {
		writeErr(w, errStatus(http.StatusInternalServerError, "password check failed"))
		return
	}
	if !ok {
		s.Auth.RecordFailure(client)
		writeErr(w, errStatus(http.StatusUnauthorized, "wrong password"))
		return
	}
	s.Auth.RecordSuccess(client)
	tok, _, err := s.Auth.NewSession()
	if err != nil {
		writeErr(w, errStatus(http.StatusInternalServerError, "cannot create session"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: s.secureCookie(r), MaxAge: int(s.cfg().AuthMax.Seconds())})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) apiLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		_ = s.Auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
