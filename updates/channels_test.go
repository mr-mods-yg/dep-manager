package updates

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestChannelDefaults(t *testing.T) {
	for _, test := range []struct{ version, channel string }{
		{"1.0.0-alpha.1", "nightly"},
		{"1.0.0", "stable"}, {"0.0.6-nightly.8", "nightly"},
	} {
		if got := New(test.version, nil).GetAppInfo().Channel; got != test.channel {
			t.Errorf("%s defaulted to %s, want %s", test.version, got, test.channel)
		}
	}
	if _, err := newChannelProvider("beta", ""); err == nil {
		t.Fatal("removed beta channel was accepted")
	}
}

func TestChannelsChooseHighestSemVerAcrossPages(t *testing.T) {
	var server *httptest.Server
	sum := sha256.Sum256([]byte("installer"))
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/checksums" {
			fmt.Fprintf(w, "%x  depmanager-amd64-installer.exe\n", sum)
			return
		}
		if r.URL.Path != "/repos/"+releaseRepository+"/releases" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Error("channel lookup did not request a full release page")
		}
		page := r.URL.Query().Get("page")
		tags := []string{"v1.3.0-alpha.2", "v88.0.0-beta.10", "v77.0.0-alpha.1", "v76.0.0", "v1.1.0", "v1.4.0-rc.1", "v99.0.0", "v01.9.0", "v9.0.0-alpha.01"}
		switch page {
		case "1":
			w.Header().Set("Link", "<"+server.URL+"/repos/"+releaseRepository+"/releases?page=2>; rel=\"next\"")
		case "2":
			tags = []string{"v1.0.0", "v1.2.0-beta.2", "v1.3.0-alpha.10"}
		default:
			t.Errorf("unexpected release page: %s", page)
		}
		releases := []map[string]any{}
		for _, tag := range tags {
			releases = append(releases, map[string]any{
				"tag_name": tag, "draft": tag == "v99.0.0",
				"prerelease": tag == "v76.0.0" || versionChannel(tag) != "stable" && tag != "v77.0.0-alpha.1",
				"assets": []map[string]any{
					{"name": "depmanager-amd64-installer.exe", "browser_download_url": server.URL + "/installer"},
					{"name": "SHA256SUMS", "browser_download_url": server.URL + "/checksums"},
				},
			})
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	for _, test := range []struct{ channel, version string }{
		{"nightly", "1.3.0-alpha.10"}, {"stable", "1.1.0"},
	} {
		t.Run(test.channel, func(t *testing.T) {
			provider, err := newChannelProvider(test.channel, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			release, err := (verifiedProvider{provider}).Check(context.Background(), updater.CheckRequest{
				CurrentVersion: "0.0.6-nightly.8", Platform: "windows", Arch: "amd64",
			})
			if err != nil {
				t.Fatal(err)
			}
			if release == nil || release.Version != test.version {
				t.Fatalf("got %+v, want %s", release, test.version)
			}
		})
	}
}
