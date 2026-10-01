// Package config reads baton's flat config file, the per-machine paths file
// and resolves the directories baton works with.
package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

// Dirs are the locations baton reads and writes.
type Dirs struct {
	Home   string // user home ($HOME first, so Termux works without /etc/passwd)
	Config string // holds config and paths
	State  string // per-machine state: password, sessions, pid, log, locks
}

// Home returns the user's home directory.
func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// ResolveDirs applies BATON_CONFIG_DIR / BATON_STATE overrides.
func ResolveDirs() Dirs {
	home := Home()
	d := Dirs{
		Home:   home,
		Config: filepath.Join(home, ".config", "baton"),
		State:  filepath.Join(home, ".local", "state", "baton"),
	}
	if v := os.Getenv("BATON_CONFIG_DIR"); v != "" {
		d.Config = ExpandHome(v, home)
	}
	if v := os.Getenv("BATON_STATE"); v != "" {
		d.State = ExpandHome(v, home)
	}
	return d
}

// ConfigFile is the path of the config file.
func (d Dirs) ConfigFile() string { return filepath.Join(d.Config, "config") }

// PathsFile is the path of the per-machine paths file.
func (d Dirs) PathsFile() string { return filepath.Join(d.Config, "paths") }

// ExpandHome expands a leading ~ to home.
func ExpandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Agent is a named command template.
type Agent struct {
	Name     string
	Template string
}

// Config is the parsed config file with defaults applied.
type Config struct {
	Root string
	// ObsidianVault enables the Obsidian integration; Root is then
	// <ObsidianVault>/<ObsidianFolder>.
	ObsidianVault  string
	ObsidianFolder string
	Keep           int
	Host           string
	Port           int
	PublicURL      string
	Auth           bool
	AuthIdle       time.Duration
	AuthMax        time.Duration
	DefaultAgent   string
	Agents         []Agent // in config order
	// BuiltinAgents: no agent.<name> line, Agents holds DefaultAgents.
	BuiltinAgents bool
	Warnings      []string
}

// DefaultAgents are used when the config defines no agent.<name>.
var DefaultAgents = []Agent{
	{"claude", `claude "/pickup {task}"`},
	{"opencode", `opencode --prompt "/pickup {task}"`},
}

// Defaults returns the config used when no file exists.
func Defaults(home string) Config {
	return Config{
		Root:           filepath.Join(home, ".local", "share", "baton"),
		Keep:           10,
		ObsidianFolder: "baton",
		Host:           "127.0.0.1",
		Port:           8765,
		AuthIdle:       7 * 24 * time.Hour,
		AuthMax:        30 * 24 * time.Hour,
	}
}

// Entry is one key = value line.
type Entry struct {
	Key, Value string
	Line       int
}

// ParseEntries parses `key = value` lines. `#` starts a comment at line start
// or after whitespace outside quotes. CRLF is accepted.
func ParseEntries(data []byte) ([]Entry, []string) {
	var out []Entry
	var warns []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(stripComment(strings.TrimRight(sc.Text(), "\r")))
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			warns = append(warns, fmt.Sprintf("config line %d: expected key = value", n))
			continue
		}
		out = append(out, Entry{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v), Line: n})
	}
	return out, warns
}

func stripComment(s string) string {
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
			return s[:i]
		}
	}
	return s
}

// Load reads the config file in dirs; a missing file yields defaults.
func Load(d Dirs) (Config, error) {
	c := Defaults(d.Home)
	data, err := os.ReadFile(d.ConfigFile())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, err
	}
	entries, warns := ParseEntries(data)
	c.Warnings = warns
	var rootSet, folderSet bool
	for _, e := range entries {
		if err := c.apply(e, d.Home); err != nil {
			c.Warnings = append(c.Warnings, fmt.Sprintf("config line %d: %v", e.Line, err))
		}
		rootSet = rootSet || e.Key == "root" && e.Value != ""
		folderSet = folderSet || e.Key == "obsidian.folder"
	}
	if c.ObsidianVault != "" {
		if rootSet {
			return c, fmt.Errorf("%s: root and obsidian.vault are both set; keep one (with obsidian.vault the root is <vault>/<obsidian.folder>)", d.ConfigFile())
		}
		folder, err := cleanFolder(c.ObsidianFolder)
		if err != nil {
			return c, fmt.Errorf("%s: obsidian.folder: %v", d.ConfigFile(), err)
		}
		c.ObsidianFolder = folder
		c.Root = filepath.Join(c.ObsidianVault, filepath.FromSlash(folder))
		if old := Defaults(d.Home).Root; os.Getenv("BATON_ROOT") == "" && hasEntries(old) && !exists(c.Root) {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s does not exist yet but %s has data; move it: mv %s %s", c.Root, old, old, c.Root))
		}
	} else if folderSet {
		c.Warnings = append(c.Warnings, "obsidian.folder has no effect without obsidian.vault")
	}
	if len(c.Agents) == 0 {
		c.Agents = append(c.Agents, DefaultAgents...)
		c.BuiltinAgents = true
	}
	if c.DefaultAgent == "" {
		c.DefaultAgent = c.Agents[0].Name
	}
	if v := os.Getenv("BATON_ROOT"); v != "" {
		c.Root = ExpandHome(v, d.Home)
	}
	return c, nil
}

// ObsidianURL returns an obsidian://open link to file, or "" when the
// integration is off or file is outside the vault (BATON_ROOT elsewhere).
// The vault name Obsidian uses is its folder name.
func (c Config) ObsidianURL(file string) string {
	if c.ObsidianVault == "" {
		return ""
	}
	rel, err := filepath.Rel(c.ObsidianVault, file)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	esc := func(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }
	return "obsidian://open?vault=" + esc(filepath.Base(c.ObsidianVault)) + "&file=" + esc(filepath.ToSlash(rel))
}

// cleanFolder checks obsidian.folder: a relative path inside the vault, not
// the vault itself (baton would take every vault folder for a project).
func cleanFolder(f string) (string, error) {
	f = strings.ReplaceAll(f, `\`, "/")
	if strings.HasPrefix(f, "/") || filepath.VolumeName(f) != "" || strings.HasPrefix(f, "~") {
		return "", fmt.Errorf("want a path relative to the vault, got %q", f)
	}
	for _, part := range strings.Split(f, "/") {
		if part == ".." {
			return "", fmt.Errorf("must stay inside the vault, got %q", f)
		}
	}
	if f = path.Clean("/" + f)[1:]; f == "" {
		return "", fmt.Errorf("must name a folder inside the vault")
	}
	return f, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func hasEntries(dir string) bool {
	es, err := os.ReadDir(dir)
	return err == nil && len(es) > 0
}

func (c *Config) apply(e Entry, home string) error {
	v := e.Value
	switch e.Key {
	case "root":
		if v != "" {
			c.Root = ExpandHome(unquote(v), home)
		}
	case "obsidian.vault":
		c.ObsidianVault = ""
		if v := unquote(v); v != "" {
			c.ObsidianVault = filepath.Clean(ExpandHome(v, home))
		}
	case "obsidian.folder":
		c.ObsidianFolder = unquote(v)
	case "keep":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("keep: want a non-negative integer, got %q", v)
		}
		c.Keep = n
	case "dashboard.host":
		if v != "" {
			c.Host = v
		}
	case "dashboard.port":
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("dashboard.port: invalid port %q", v)
		}
		c.Port = n
	case "dashboard.public_url":
		c.PublicURL = strings.TrimRight(v, "/")
	case "dashboard.auth":
		switch strings.ToLower(v) {
		case "on", "true", "yes", "1":
			c.Auth = true
		case "off", "false", "no", "0", "":
			c.Auth = false
		default:
			return fmt.Errorf("dashboard.auth: want on|off, got %q", v)
		}
	case "dashboard.auth.idle", "dashboard.auth.max":
		dur, err := ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s: %v", e.Key, err)
		}
		if e.Key == "dashboard.auth.idle" {
			c.AuthIdle = dur
		} else {
			c.AuthMax = dur
		}
	case "agent.default":
		c.DefaultAgent = v
	default:
		if name, ok := strings.CutPrefix(e.Key, "agent."); ok && name != "" {
			for i := range c.Agents {
				if c.Agents[i].Name == name {
					c.Agents[i].Template = v
					return nil
				}
			}
			c.Agents = append(c.Agents, Agent{Name: name, Template: v})
			return nil
		}
		return fmt.Errorf("unknown key %q", e.Key)
	}
	return nil
}

// Agent returns the template for name ("" means the default agent).
func (c Config) Agent(name string) (Agent, error) {
	if name == "" {
		name = c.DefaultAgent
	}
	for _, a := range c.Agents {
		if a.Name == name {
			return a, nil
		}
	}
	names := make([]string, len(c.Agents))
	for i, a := range c.Agents {
		names[i] = a.Name
	}
	return Agent{}, fmt.Errorf("unknown agent %q (configured: %s)", name, strings.Join(names, ", "))
}

// ParseDuration accepts Go durations plus a `d` (day) suffix, e.g. 7d, 1d12h.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var total time.Duration
	if i := strings.Index(s, "d"); i > 0 {
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		total = time.Duration(n) * 24 * time.Hour
		s = s[i+1:]
		if s == "" {
			return total, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return total + d, nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// SetKey sets key = value in the config file, replacing the first existing
// line for key or appending one. Other lines and comments are preserved.
func SetKey(file, key, value string) error {
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if text == "" {
		lines = nil
	}
	newLine := key + " = " + value
	done := false
	for i, l := range lines {
		k, _, ok := strings.Cut(stripComment(l), "=")
		if ok && strings.TrimSpace(k) == key {
			lines[i] = newLine
			done = true
			break
		}
	}
	if !done {
		lines = append(lines, newLine)
	}
	return fsutil.WriteFileAtomic(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// DefaultFile is what `baton config init` writes: every key commented out at
// its built-in value, so later defaults still apply. `baton integrate` appends
// the agent.<name> lines.
const DefaultFile = `# baton config: flat "key = value"; # starts a comment.
# Uncomment a line to change it; the values shown are the defaults.

# root = ~/.local/share/baton
# keep = 10

# Obsidian: keep the handoffs in a vault folder instead of root (set only
# one of root and obsidian.vault); root becomes <vault>/<obsidian.folder>.
# obsidian.vault =
# obsidian.folder = baton

# dashboard.host = 127.0.0.1
# dashboard.port = 8765
# dashboard.public_url =
# dashboard.auth = off
# dashboard.auth.idle = 7d
# dashboard.auth.max = 30d

# Agents for "baton pickup" and the dashboard's copy buttons; "baton integrate
# NAME" adds its line. Without agent.* lines the CLI falls back to claude and
# opencode and the dashboard shows no pickup buttons.
# agent.default = claude
`

// EnsureFile writes DefaultFile unless file exists. It reports whether it
// created the file.
func EnsureFile(file string) (bool, error) {
	if _, err := os.Stat(file); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return false, err
	}
	return true, fsutil.WriteFileAtomic(file, []byte(DefaultFile), 0o644)
}

// DefaultAgentTemplate returns the built-in template for name.
func DefaultAgentTemplate(name string) (string, bool) {
	for _, a := range DefaultAgents {
		if a.Name == name {
			return a.Template, true
		}
	}
	return "", false
}

// FileValue returns the value of the first line for key in file.
func FileValue(file, key string) (string, bool, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	entries, _ := ParseEntries(data)
	for _, e := range entries {
		if e.Key == key {
			return e.Value, true, nil
		}
	}
	return "", false, nil
}

// RemoveKey deletes every line for key from file; comments stay. It reports
// whether a line was removed.
func RemoveKey(file, key string) (bool, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	var keep []string
	removed := false
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if k, _, ok := strings.Cut(stripComment(l), "="); ok && strings.TrimSpace(k) == key {
			removed = true
			continue
		}
		keep = append(keep, l)
	}
	if !removed {
		return false, nil
	}
	return true, fsutil.WriteFileAtomic(file, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}
