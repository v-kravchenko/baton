package config

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

// Paths maps project key → directory on this machine.
type Paths map[string]string

// HomeKey is the project of the user's home directory. Its directory is
// always the home directory of this machine: never read from or written to
// the paths file.
const HomeKey = "home"

// ParsePaths parses `key=path` lines. The first `=` separates key and path,
// so Windows paths (C:\...) work as is. CRLF and `#` comment lines are accepted.
func ParsePaths(data []byte) Paths {
	p := Paths{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimRight(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if ok && k != "" && v != "" {
			p[k] = v
		}
	}
	return p
}

// LoadPaths reads the paths file (a missing file is empty) and adds HomeKey.
func LoadPaths(d Dirs) (Paths, error) {
	p, err := readPaths(d)
	if d.Home != "" {
		p[HomeKey] = d.Home
	}
	return p, err
}

func readPaths(d Dirs) (Paths, error) {
	data, err := os.ReadFile(d.PathsFile())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Paths{}, err
	}
	p := ParsePaths(data)
	delete(p, HomeKey)
	return p, nil
}

// Keys returns sorted keys.
func (p Paths) Keys() []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Format renders the paths file content.
func (p Paths) Format() []byte {
	var b strings.Builder
	for _, k := range p.Keys() {
		b.WriteString(k + "=" + p[k] + "\n")
	}
	return []byte(b.String())
}

// UpdatePaths applies fn to the paths file under a lock and writes it back if
// fn reports a change.
func UpdatePaths(d Dirs, fn func(Paths) bool) error {
	unlock, err := fsutil.Lock(filepath.Join(d.State, "locks", "paths.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	p, err := readPaths(d)
	if err != nil {
		return err
	}
	if !fn(p) {
		return nil
	}
	return fsutil.WriteFileAtomic(d.PathsFile(), p.Format(), 0o644)
}

// Remember records key → dir if it differs from the current entry; HomeKey
// is never recorded.
func Remember(d Dirs, key, dir string) error {
	if key == HomeKey {
		return nil
	}
	cur, _ := LoadPaths(d)
	if cur[key] == dir {
		return nil
	}
	return UpdatePaths(d, func(p Paths) bool {
		if p[key] == dir {
			return false
		}
		p[key] = dir
		return true
	})
}
