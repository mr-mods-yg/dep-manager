package updates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

var versionCore = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:[-+]|$)`)

func canonicalVersion(version string) string { return "v" + strings.TrimPrefix(version, "v") }

func versionChannel(version string) string {
	version = canonicalVersion(version)
	if !versionCore.MatchString(version) || !semver.IsValid(version) {
		return ""
	}
	prerelease := semver.Prerelease(version)
	if prerelease == "" {
		return "stable"
	}
	switch strings.Split(strings.TrimPrefix(prerelease, "-"), ".")[0] {
	case "alpha", "nightly": // Keep existing installations on the nightly channel.
		return "nightly"
	case "beta":
		return "beta"
	}
	return ""
}

func allowsRelease(channel, version string, prerelease bool) bool {
	actual := versionChannel(version)
	if actual == "" || (actual == "stable") == prerelease {
		return false
	}
	// A channel includes more mature releases, allowing promotion without
	// changing a saved preference. Beta never installs a nightly build.
	return actual == "stable" || actual == channel || channel == "nightly" && actual == "beta"
}

// Filter GitHub's release list before the framework chooses an asset. Its
// provider still performs checksum verification and the actual download.
type releaseChannelTransport struct {
	channel string
	base    http.RoundTripper
}

func (t releaseChannelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method != http.MethodGet || request.URL.Path != "/repos/"+releaseRepository+"/releases" {
		return t.base.RoundTrip(request)
	}
	var best json.RawMessage
	bestVersion := ""
	for page := 1; ; page++ {
		next := request.Clone(request.Context())
		url := *request.URL
		query := url.Query()
		query.Set("per_page", "100")
		query.Set("page", strconv.Itoa(page))
		url.RawQuery = query.Encode()
		next.URL = &url
		response, err := t.base.RoundTrip(next)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			return response, nil
		}
		var releases []json.RawMessage
		err = json.NewDecoder(response.Body).Decode(&releases)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode GitHub channel releases: %w", err)
		}
		for _, release := range releases {
			var info struct {
				TagName    string `json:"tag_name"`
				Draft      bool   `json:"draft"`
				Prerelease bool   `json:"prerelease"`
			}
			if err := json.Unmarshal(release, &info); err != nil {
				return nil, fmt.Errorf("decode GitHub channel release: %w", err)
			}
			if info.Draft || !allowsRelease(t.channel, info.TagName, info.Prerelease) {
				continue
			}
			if best == nil || semver.Compare(canonicalVersion(info.TagName), canonicalVersion(bestVersion)) > 0 {
				best, bestVersion = release, info.TagName
			}
		}
		if strings.Contains(response.Header.Get("Link"), `rel="next"`) {
			continue
		}
		selected := []json.RawMessage{}
		if best != nil {
			selected = append(selected, best)
		}
		body, err := json.Marshal(selected)
		if err != nil {
			return nil, err
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		response.Header.Set("Content-Length", strconv.Itoa(len(body)))
		response.Header.Del("Link")
		return response, nil
	}
}
