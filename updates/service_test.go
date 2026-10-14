package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	githubprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

type testHost struct{}

func (testHost) Emit(string, ...any) bool         { return false }
func (testHost) OnEvent(string, func(any)) func() { return func() {} }
func (testHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	panic("updater should be headless")
}
func (testHost) Quit() {}

func TestAssetSelection(t *testing.T) {
	assets := []githubprovider.ReleaseAsset{
		{Name: "depmanager-arm64-installer.exe"},
		{Name: "depmanager-amd64-installer.exe"},
		{Name: "depmanager_1.0.2_amd64.deb"},
		{Name: "SHA256SUMS"},
	}
	for _, test := range []struct {
		platform, arch string
		want           int
	}{
		{"windows", "amd64", 1}, {"windows", "arm64", 0}, {"linux", "amd64", 2}, {"linux", "arm64", -1}, {"darwin", "amd64", -1},
	} {
		if got := matchUpdateAsset(updater.CheckRequest{Platform: test.platform, Arch: test.arch}, assets); got != test.want {
			t.Errorf("%s/%s matched %d, want %d", test.platform, test.arch, got, test.want)
		}
	}
}

// Exercise the real GitHub provider, checksum parser, downloader, and service
// against a local release. The installer is replaced so tests cannot update apps.
func fixtureService(t *testing.T, version, checksum string, status int, currentVersions ...string) (*Service, []byte) {
	t.Helper()
	currentVersion := "1.0.0"
	if len(currentVersions) > 0 {
		currentVersion = currentVersions[0]
	}
	payload := []byte("a verified test installer")
	filename := "depmanager-" + runtime.GOARCH + "-installer.exe"
	if runtime.GOOS == "linux" {
		filename = "depmanager_1.1.0_" + runtime.GOARCH + ".deb"
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + releaseRepository + "/releases/latest", "/repos/" + releaseRepository + "/releases":
			if status != 200 {
				w.WriteHeader(status)
				return
			}
			assets := []map[string]any{{"name": filename, "browser_download_url": server.URL + "/installer", "size": len(payload)}}
			if checksum != "missing" {
				assets = append(assets, map[string]any{"name": "SHA256SUMS", "browser_download_url": server.URL + "/checksums"})
			}
			release := map[string]any{"tag_name": version, "assets": assets, "html_url": server.URL + "/release", "prerelease": versionChannel(version) != "stable"}
			if strings.HasSuffix(r.URL.Path, "/latest") {
				if isNightlyVersion(version) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_ = json.NewEncoder(w).Encode(release)
			} else {
				// Drafts must never be installed, even if listed before a nightly.
				_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": "v99.0.0", "draft": true}, release})
			}
		case "/checksums":
			if checksum == "valid" {
				sum := sha256.Sum256(payload)
				checksum = hex.EncodeToString(sum[:])
			}
			fmt.Fprintf(w, "%s  %s\n", checksum, filename)
		case "/installer":
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	provider, err := newChannelProvider(defaultUpdateChannel(currentVersion), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	s := New(currentVersion, &sync.RWMutex{})
	s.provider = provider
	s.canInstall = true
	s.updater = updater.New(testHost{})
	if err := s.updater.Init(updater.Config{CurrentVersion: s.version, Providers: []updater.Provider{verifiedProvider{provider}}, Window: updater.WindowNone}); err != nil {
		t.Fatal(err)
	}
	s.installer = func(context.Context, string, string) error { t.Fatal("unexpected installer invocation"); return nil }
	s.quit = func() { t.Error("unexpected shutdown") }
	t.Cleanup(func() { _, _ = s.updater.Check(context.Background()) })
	return s, payload
}

func TestCheckReleases(t *testing.T) {
	for _, test := range []struct {
		name, tag, sum string
		status         int
		available      bool
		errorText      string
	}{
		{"newer", "v1.1.0", "valid", 200, true, ""},
		{"same", "v1.0.0", "missing", 200, false, ""},
		{"older", "v0.9.9", "valid", 200, false, ""},
		{"missing checksum", "v1.1.0", "missing", 200, false, "SHA256SUMS"},
		{"invalid checksum", "v1.1.0", "abcd", 200, false, "SHA256SUMS"},
		{"rate limit", "v1.1.0", "valid", 403, false, "403"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := fixtureService(t, test.tag, test.sum, test.status)
			info, err := s.CheckForUpdates("")
			if test.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), test.errorText) {
					t.Fatalf("got %v, want %q", err, test.errorText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if info.Available != test.available || info.CurrentVersion != "1.0.0" {
				t.Fatalf("unexpected info: %+v", info)
			}
		})
	}
}

func TestNightlyReleaseChecks(t *testing.T) {
	for _, test := range []struct {
		name, current, latest string
		available             bool
	}{
		{"new nightly", "1.0.4-nightly.1", "v1.0.5-nightly.2", true},
		{"same nightly", "1.0.4-nightly.1", "v1.0.4-nightly.1", false},
		{"older nightly", "1.0.5-nightly.2", "v1.0.4-nightly.1", false},
		{"stable promotion", "1.0.4-nightly.1", "v1.0.4", true},
		{"stable ignores nightly", "1.0.3", "v1.0.4-nightly.1", false},
		{"new alpha", "1.0.0-alpha.1", "v1.0.0-alpha.2", true},
		{"nightly ignores removed beta", "1.0.0-alpha.10", "v1.0.0-beta.1", false},
		{"stable excludes beta", "0.9.0", "v1.0.0-beta.2", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := fixtureService(t, test.latest, "valid", 200, test.current)
			info, err := s.CheckForUpdates("")
			if err != nil {
				t.Fatal(err)
			}
			if info.Available != test.available {
				t.Fatalf("unexpected availability: %+v", info)
			}
			if test.available && !strings.HasSuffix(info.ReleaseURL, "/release") {
				t.Fatalf("missing link to the selected release: %s", info.ReleaseURL)
			}
		})
	}
}

func TestSelectedChannelOverridesBuildChannel(t *testing.T) {
	for _, test := range []struct{ current, selected, latest string }{
		{"1.0.3", "nightly", "v1.0.4-nightly.1"},
		{"1.0.4-nightly.1", "stable", "v1.0.5"},
		{"0.0.6-nightly.8", "nightly", "v1.0.0-alpha.1"},
	} {
		t.Run(test.selected, func(t *testing.T) {
			s, _ := fixtureService(t, test.latest, "valid", 200, test.current)
			info, err := s.CheckForUpdates(test.selected)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Available || info.Channel != test.selected || s.availableChannel != test.selected {
				t.Fatalf("selected channel was not used: %+v", info)
			}
		})
	}
}

func TestChannelSwitchInvalidatesPreviousOffer(t *testing.T) {
	s, _ := fixtureService(t, "v1.0.4-nightly.1", "valid", 200, "1.0.3")
	info, err := s.CheckForUpdates("nightly")
	if err != nil || !info.Available {
		t.Fatalf("nightly was not offered: %+v, %v", info, err)
	}
	info, err = s.CheckForUpdates("stable")
	if err != nil || info.Available {
		t.Fatalf("stable channel retained the nightly offer: %+v, %v", info, err)
	}
	if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "selected channel") {
		t.Fatalf("stale nightly update could still be installed: %v", err)
	}
}

func TestFailedChannelCheckInvalidatesPreviousOffer(t *testing.T) {
	s, _ := fixtureService(t, "v1.1.0", "valid", 200)
	if _, err := s.CheckForUpdates("stable"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckForUpdates("invalid"); err == nil {
		t.Fatal("invalid channel was accepted")
	}
	if err := s.InstallUpdate(); err == nil {
		t.Fatal("previous offer remained installable after a failed check")
	}
}

func TestInstallVerifiesBeforeInstallerAndRestarts(t *testing.T) {
	s, payload := fixtureService(t, "v1.1.0", "valid", 200)
	if _, err := s.CheckForUpdates(""); err != nil {
		t.Fatal(err)
	}
	called := false
	s.installer = func(_ context.Context, path, executable string) error {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != string(payload) || executable == "" {
			t.Fatalf("invalid staged installer: %q, %v", data, err)
		}
		called = true
		return nil
	}
	quit := make(chan struct{})
	s.quit = func() { close(quit) }
	if err := s.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	if !called || !s.installing {
		t.Fatal("installer was not launched")
	}
	if s.operations.TryRLock() {
		s.operations.RUnlock()
		t.Fatal("mutations were not blocked during restart")
	}
	select {
	case <-quit:
	case <-time.After(3 * time.Second):
		t.Fatal("no restart requested")
	}
}

func TestInstallRejectsCorruptDownload(t *testing.T) {
	s, _ := fixtureService(t, "v1.1.0", strings.Repeat("0", 64), 200)
	if _, err := s.CheckForUpdates(""); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("got %v", err)
	}
	if !s.operations.TryLock() {
		t.Fatal("package operations remained blocked after failure")
	}
	s.operations.Unlock()
}

func TestInstallFailureKeepsAppOpenAndAllowsRetry(t *testing.T) {
	s, _ := fixtureService(t, "v1.1.0", "valid", 200)
	if _, err := s.CheckForUpdates(""); err != nil {
		t.Fatal(err)
	}
	s.installer = func(context.Context, string, string) error { return errors.New("cancelled") }
	if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("got %v", err)
	}
	if s.installing {
		t.Fatal("failed update marked as installing")
	}
	if !s.operations.TryLock() {
		t.Fatal("mutations remained blocked")
	}
	s.operations.Unlock()
}

func TestUpdateWaitsForPackageWork(t *testing.T) {
	s, _ := fixtureService(t, "v1.1.0", "valid", 200)
	if _, err := s.CheckForUpdates(""); err != nil {
		t.Fatal(err)
	}
	s.operations.RLock()
	defer s.operations.RUnlock()
	if err := s.InstallUpdate(); err == nil || !strings.Contains(err.Error(), "package operations") {
		t.Fatalf("got %v", err)
	}
}

func TestDevelopmentInstallDisabled(t *testing.T) {
	s := New("1.0.0", nil)
	s.canInstall = false
	if err := s.InstallUpdate(); err == nil {
		t.Fatal("installation should be disabled")
	}
}
