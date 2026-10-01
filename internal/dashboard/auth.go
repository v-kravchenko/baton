package dashboard

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/scrypt"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

// scrypt parameters for new password hashes.
const (
	scryptN      = 1 << 15
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32
	minPassword  = 8
)

// Auth manages the dashboard password and sessions in the state directory.
type Auth struct {
	StateDir string
	Idle     time.Duration
	Max      time.Duration
	Now      func() time.Time

	mu       sync.Mutex
	failures map[string]*failure
}

type failure struct {
	count int
	until time.Time
	last  time.Time
}

// maxFailures bounds the failure table; beyond it, entries idle for lockMax
// are dropped so many client addresses cannot grow it without limit.
const maxFailures = 1024

type session struct {
	Created time.Time `json:"created"`
	Last    time.Time `json:"last"`
	CSRF    string    `json:"csrf"`
}

func (a *Auth) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Auth) setLimits(idle, max time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Idle, a.Max = idle, max
}

func (a *Auth) passwordFile() string { return filepath.Join(a.StateDir, "password") }
func (a *Auth) sessionsFile() string { return filepath.Join(a.StateDir, "sessions") }

// HasPassword reports whether a password is set.
func (a *Auth) HasPassword() bool {
	_, err := os.Stat(a.passwordFile())
	return err == nil
}

// SetPassword stores a scrypt hash of pw and drops all sessions.
func (a *Auth) SetPassword(pw string) error {
	if len([]rune(pw)) < minPassword {
		return fmt.Errorf("password must be at least %d characters", minPassword)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := scrypt.Key([]byte(pw), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return err
	}
	enc := base64.RawStdEncoding
	line := fmt.Sprintf("scrypt$%d$%d$%d$%s$%s\n", scryptN, scryptR, scryptP, enc.EncodeToString(salt), enc.EncodeToString(key))
	if err := fsutil.WriteFileAtomic(a.passwordFile(), []byte(line), 0o600); err != nil {
		return err
	}
	return a.LogoutAll()
}

// CheckPassword verifies pw against the stored hash.
func (a *Auth) CheckPassword(pw string) (bool, error) {
	data, err := os.ReadFile(a.passwordFile())
	if err != nil {
		return false, err
	}
	parts := strings.Split(strings.TrimSpace(string(data)), "$")
	if len(parts) != 6 || parts[0] != "scrypt" {
		return false, errors.New("password file: unknown format")
	}
	n, err1 := strconv.Atoi(parts[1])
	r, err2 := strconv.Atoi(parts[2])
	p, err3 := strconv.Atoi(parts[3])
	enc := base64.RawStdEncoding
	salt, err4 := enc.DecodeString(parts[4])
	want, err5 := enc.DecodeString(parts[5])
	if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
		return false, fmt.Errorf("password file: %v", err)
	}
	got, err := scrypt.Key([]byte(pw), salt, n, r, p, len(want))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// Lockout: after lockAfter failures from one client, wait lockBase doubling
// up to lockMax.
const (
	lockAfter = 5
	lockBase  = 30 * time.Second
	lockMax   = 15 * time.Minute
)

// Locked returns how long a client must wait before trying again.
func (a *Auth) Locked(client string) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if f := a.failures[client]; f != nil && f.until.After(a.now()) {
		return f.until.Sub(a.now())
	}
	return 0
}

// RecordFailure counts a failed login.
func (a *Auth) RecordFailure(client string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.failures == nil {
		a.failures = map[string]*failure{}
	}
	now := a.now()
	if len(a.failures) >= maxFailures {
		for k, f := range a.failures {
			if now.Sub(f.last) > lockMax {
				delete(a.failures, k)
			}
		}
	}
	f := a.failures[client]
	if f == nil {
		f = &failure{}
		a.failures[client] = f
	}
	f.count++
	f.last = now
	if f.count >= lockAfter {
		d := lockBase << (f.count - lockAfter)
		if d > lockMax || d <= 0 {
			d = lockMax
		}
		f.until = now.Add(d)
	}
}

// RecordSuccess clears a client's failures.
func (a *Auth) RecordSuccess(client string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.failures, client)
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Auth) loadSessions() (map[string]*session, error) {
	data, err := os.ReadFile(a.sessionsFile())
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]*session{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]*session{}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &m); err != nil {
			return map[string]*session{}, nil
		}
	}
	return m, nil
}

func (a *Auth) saveSessions(m map[string]*session) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(a.sessionsFile(), append(data, '\n'), 0o600)
}

func (a *Auth) expired(s *session, now time.Time) bool {
	return now.Sub(s.Last) > a.Idle || now.Sub(s.Created) > a.Max
}

// NewSession creates a session and returns its token and CSRF token.
func (a *Auth) NewSession() (token, csrf string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.loadSessions()
	if err != nil {
		return "", "", err
	}
	now := a.now()
	for k, s := range m {
		if a.expired(s, now) {
			delete(m, k)
		}
	}
	token, csrf = randomToken(), randomToken()
	m[hashToken(token)] = &session{Created: now, Last: now, CSRF: csrf}
	return token, csrf, a.saveSessions(m)
}

// Session validates a token, refreshes its idle timer and returns its CSRF
// token. Sessions are re-read from disk so `baton auth logout-all` applies
// to a running server at once.
func (a *Auth) Session(token string) (csrf string, ok bool) {
	if token == "" {
		return "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.loadSessions()
	if err != nil {
		return "", false
	}
	k := hashToken(token)
	s := m[k]
	now := a.now()
	if s == nil {
		return "", false
	}
	if a.expired(s, now) {
		delete(m, k)
		_ = a.saveSessions(m)
		return "", false
	}
	if now.Sub(s.Last) > time.Minute {
		s.Last = now
		_ = a.saveSessions(m)
	}
	return s.CSRF, true
}

// Logout drops one session.
func (a *Auth) Logout(token string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, err := a.loadSessions()
	if err != nil {
		return err
	}
	delete(m, hashToken(token))
	return a.saveSessions(m)
}

// LogoutAll drops every session.
func (a *Auth) LogoutAll() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := os.Remove(a.sessionsFile())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// SessionCount returns live sessions.
func (a *Auth) SessionCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, _ := a.loadSessions()
	n := 0
	for _, s := range m {
		if !a.expired(s, a.now()) {
			n++
		}
	}
	return n
}
