package update

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadAndReplace(t *testing.T) {
	bin := []byte("new baton binary")
	sum := sha256.Sum256(bin)
	name := AssetName(runtime.GOOS, runtime.GOARCH)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[{"name":%q,"browser_download_url":"%s/bin"},{"name":"checksums.txt","browser_download_url":"%s/sums"}]}`, name, srv.URL, srv.URL)
		case "/bin":
			w.Write(bin)
		case "/sums":
			fmt.Fprintf(w, "%s  %s\n%s  other\n", hex.EncodeToString(sum[:]), name, strings.Repeat("0", 64))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("BATON_UPDATE_URL", srv.URL)
	c := srv.Client()
	rel, err := Latest(c)
	if err != nil || rel.Tag != "v1.2.3" {
		t.Fatalf("latest = %+v %v", rel, err)
	}
	if !Same("1.2.3", rel.Tag) || Same("v1.2.2", rel.Tag) {
		t.Error("Same")
	}
	got, err := Download(c, rel)
	if err != nil || string(got) != string(bin) {
		t.Fatalf("download = %q %v", got, err)
	}
	// Tampered binary is rejected.
	rel.Assets[0].URL = srv.URL + "/sums"
	if _, err := Download(c, rel); err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Errorf("tampered download: %v", err)
	}
	exe := filepath.Join(t.TempDir(), "baton")
	os.WriteFile(exe, []byte("old"), 0o755)
	if err := Replace(exe, bin); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exe); string(data) != string(bin) {
		t.Errorf("exe = %q", data)
	}
}

func TestAssetName(t *testing.T) {
	for _, c := range [][3]string{{"linux", "arm", "baton_linux_armv7"}, {"windows", "arm64", "baton_windows_arm64.exe"}, {"darwin", "amd64", "baton_darwin_amd64"}} {
		if got := AssetName(c[0], c[1]); got != c[2] {
			t.Errorf("%v = %s", c, got)
		}
	}
}
