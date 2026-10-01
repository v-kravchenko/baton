// Package update replaces the running binary with the latest GitHub release
// for this OS/architecture after checking its SHA256 from checksums.txt.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/v-kravchenko/baton/internal/fsutil"
)

// Repo is the GitHub repository releases come from.
const Repo = "v-kravchenko/baton"

// APIBase returns the releases API base (BATON_UPDATE_URL overrides it).
func APIBase() string {
	if v := os.Getenv("BATON_UPDATE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.github.com/repos/" + Repo
}

// AssetName is the release file for goos/goarch (see .goreleaser.yaml).
func AssetName(goos, goarch string) string {
	name := "baton_" + goos + "_" + goarch
	if goarch == "arm" {
		name += "v7"
	}
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Release is the subset of the GitHub release JSON baton uses.
type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// Asset is a release file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func (r *Release) asset(name string) *Asset {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

// Client returns an HTTP client. On Termux, where /etc/resolv.conf is
// missing, DNS goes to the nameserver in $PREFIX/etc/resolv.conf.
func Client() *http.Client {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if ns := termuxNameserver(); ns != "" {
		dialer.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, net.JoinHostPort(ns, "53"))
			},
		}
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = dialer.DialContext
	return &http.Client{Transport: tr, Timeout: 5 * time.Minute}
}

func termuxNameserver() string {
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		return ""
	}
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(prefix, "etc", "resolv.conf"))
	if err != nil {
		return "8.8.8.8"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "nameserver" {
			return fields[1]
		}
	}
	return "8.8.8.8"
}

func get(c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "baton-update")
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 256<<20))
}

// Latest fetches the latest release.
func Latest(c *http.Client) (*Release, error) {
	data, err := get(c, APIBase()+"/releases/latest")
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("release JSON: %v", err)
	}
	if r.Tag == "" {
		return nil, fmt.Errorf("release JSON has no tag_name")
	}
	return &r, nil
}

// Same reports whether version and tag name the same release.
func Same(version, tag string) bool {
	return strings.TrimPrefix(version, "v") == strings.TrimPrefix(tag, "v")
}

// Checksum finds name in a checksums.txt body.
func Checksum(sums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), true
		}
	}
	return "", false
}

// Download fetches this platform's binary and verifies it.
func Download(c *http.Client, r *Release) ([]byte, error) {
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	a := r.asset(name)
	if a == nil {
		return nil, fmt.Errorf("release %s has no %s", r.Tag, name)
	}
	sa := r.asset("checksums.txt")
	if sa == nil {
		return nil, fmt.Errorf("release %s has no checksums.txt", r.Tag)
	}
	sums, err := get(c, sa.URL)
	if err != nil {
		return nil, err
	}
	want, ok := Checksum(sums, name)
	if !ok {
		return nil, fmt.Errorf("checksums.txt has no entry for %s", name)
	}
	bin, err := get(c, a.URL)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return nil, fmt.Errorf("SHA256 mismatch for %s: got %s, want %s", name, got, want)
	}
	return bin, nil
}

// Replace atomically swaps the executable at exe for bin. On Windows the
// running .exe cannot be overwritten, so it is renamed to .old first.
func Replace(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, "."+filepath.Base(exe)+".new")
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return fmt.Errorf("write %s: %v (is the install directory writable?)", tmp, err)
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := fsutil.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return err
		}
		if err := fsutil.Rename(tmp, exe); err != nil {
			_ = fsutil.Rename(old, exe)
			return err
		}
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// CleanupOld removes a leftover .old executable from a Windows update.
func CleanupOld() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}
