package updates

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	githubprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const releaseRepository = "mr-mods-yg/dep-manager"

type AppUpdateInfo struct {
	CurrentVersion   string `json:"currentVersion"`
	Channel          string `json:"channel"`
	LatestVersion    string `json:"latestVersion,omitempty"`
	Available        bool   `json:"available"`
	CanInstall       bool   `json:"canInstall"`
	Message          string `json:"message"`
	ReleaseURL       string `json:"releaseURL"`
	LastError        string `json:"lastError,omitempty"`
	DevelopmentBuild bool   `json:"developmentBuild"`
}

type Service struct {
	version          string
	defaultChannel   string
	canInstall       bool
	operations       *sync.RWMutex
	installer        func(context.Context, string, string) error
	updater          *updater.Updater
	provider         *channelProvider
	quit             func()
	mu               sync.Mutex
	installing       bool
	availableChannel string
}

func New(version string, operations *sync.RWMutex, channels ...string) *Service {
	channel := defaultUpdateChannel(version)
	if len(channels) > 0 {
		switch channels[0] {
		case "stable", "nightly":
			channel = channels[0]
		case "beta":
			channel = "nightly"
		}
	}
	return &Service{
		version:        version,
		defaultChannel: channel,
		canInstall:     appUpdatesEnabled && (runtime.GOOS == "windows" || runtime.GOOS == "linux") && runtime.GOARCH == "amd64",
		operations:     operations,
		installer:      installAppUpdate,
	}
}

// The framework permits unsigned downloads. This app requires the release's
// checksum sidecar before offering any update for automatic installation.
type verifiedProvider struct{ updater.Provider }

func (p verifiedProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	release, err := p.Provider.Check(ctx, req)
	if err != nil || release == nil {
		return release, err
	}
	if release.Verification == nil || release.Verification.DigestAlgo != "sha256" || len(release.Verification.Digest) != sha256.Size {
		return nil, errors.New("this release has no valid SHA256SUMS entry; download it manually from GitHub or wait for a verified release")
	}
	return release, nil
}

type channelProvider struct {
	updater.Provider
	stable, nightly updater.Provider
}

func (p *channelProvider) selectChannel(channel string) error {
	switch channel {
	case "stable":
		p.Provider = p.stable
	case "nightly":
		p.Provider = p.nightly
	default:
		return errors.New("choose the stable or nightly update channel")
	}
	return nil
}

func newChannelProvider(channel, baseURL string) (*channelProvider, error) {
	stableConfig := releaseProviderConfig("stable")
	stableConfig.BaseURL = baseURL
	stable, err := githubprovider.New(stableConfig)
	if err != nil {
		return nil, err
	}
	nightlyConfig := stableConfig
	nightlyConfig.Prerelease = true
	nightly, err := githubprovider.New(nightlyConfig)
	if err != nil {
		return nil, err
	}
	p := &channelProvider{stable: stable, nightly: nightly}
	if err := p.selectChannel(channel); err != nil {
		return nil, err
	}
	return p, nil
}

func releaseProviderConfig(channel string) githubprovider.Config {
	return githubprovider.Config{
		Repository:    releaseRepository,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  matchUpdateAsset,
		Prerelease:    channel == "nightly",
	}
}

func isNightlyVersion(version string) bool {
	return strings.Contains(version, "-nightly.")
}

func defaultUpdateChannel(version string) string {
	if isNightlyVersion(version) {
		return "nightly"
	}
	return "stable"
}

func Configure(s *Service, runtimeUpdater *updater.Updater, quit func()) error {
	provider, err := newChannelProvider(s.defaultChannel, "")
	if err != nil {
		return err
	}
	if err := runtimeUpdater.Init(updater.Config{
		CurrentVersion: s.version,
		Providers:      []updater.Provider{verifiedProvider{provider}},
		Window:         updater.WindowNone,
	}); err != nil {
		return err
	}
	s.updater = runtimeUpdater
	s.provider = provider
	s.quit = quit
	return nil
}

func matchUpdateAsset(req updater.CheckRequest, assets []githubprovider.ReleaseAsset) int {
	for i, asset := range assets {
		name := strings.ToLower(asset.Name)
		switch req.Platform {
		case "windows":
			if name == "depmanager-"+req.Arch+"-installer.exe" {
				return i
			}
		case "linux":
			if strings.HasPrefix(name, "depmanager_") && strings.HasSuffix(name, "_"+req.Arch+".deb") {
				return i
			}
		}
	}
	return -1
}

func (s *Service) GetAppInfo() AppUpdateInfo {
	info := AppUpdateInfo{
		CurrentVersion:   s.version,
		Channel:          s.defaultChannel,
		CanInstall:       s.canInstall,
		DevelopmentBuild: !appUpdatesEnabled,
		Message:          "Checks GitHub for the latest stable release.",
		ReleaseURL:       "https://github.com/" + releaseRepository + "/releases/latest",
	}
	if s.defaultChannel == "nightly" {
		info.Message = "Checks GitHub for the latest nightly prerelease."
		info.ReleaseURL = "https://github.com/" + releaseRepository + "/releases"
	}
	if !appUpdatesEnabled {
		info.Message = "Development build: updates can be checked, but installation is disabled."
	} else if !info.CanInstall {
		info.Message = "Automatic installation is available for Windows x64 and Debian/Ubuntu x64."
	}
	if data, err := os.ReadFile(updateErrorPath()); err == nil {
		info.LastError = strings.TrimSpace(string(data))
	}
	return info
}

func (s *Service) CheckForUpdates(channel string) (AppUpdateInfo, error) {
	info := s.GetAppInfo()
	if !s.mu.TryLock() {
		return info, errors.New("an update operation is already running")
	}
	defer s.mu.Unlock()
	if s.installing {
		return info, errors.New("the application is restarting to finish an update")
	}
	// The framework retains its pending release after an unsuccessful check.
	// Invalidate that offer before changing channels or checking again.
	s.availableChannel = ""
	if channel == "" {
		channel = s.defaultChannel
	}
	if err := s.provider.selectChannel(channel); err != nil {
		return info, err
	}
	info.Channel = channel
	info.ReleaseURL = "https://github.com/" + releaseRepository + "/releases"
	if channel == "stable" {
		info.ReleaseURL += "/latest"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	release, err := s.updater.Check(ctx)
	if err != nil {
		return info, fmt.Errorf("could not check GitHub releases: %w", err)
	}
	info.Message = "No newer " + channel + " release is available."
	if release != nil {
		s.availableChannel = channel
		info.LatestVersion = release.Version
		info.Available = true
		info.Message = "DepManager " + release.Version + " is available."
		if url, ok := release.Metadata["github.release.htmlURL"].(string); ok && url != "" {
			info.ReleaseURL = url
		}
	}
	if !info.CanInstall && !appUpdatesEnabled {
		info.Message += " Automatic installation is disabled in development builds."
	}
	return info, nil
}

func (s *Service) InstallUpdate() error {
	if !s.GetAppInfo().CanInstall {
		return errors.New("automatic installation is unavailable for this build")
	}
	if !s.mu.TryLock() {
		return errors.New("an update operation is already running")
	}
	defer s.mu.Unlock()
	if s.installing {
		return errors.New("the application is already restarting")
	}
	if s.availableChannel == "" {
		return errors.New("check for updates in the selected channel before installing")
	}
	if s.operations != nil {
		if !s.operations.TryLock() {
			return errors.New("wait for package operations to finish before updating DepManager")
		}
		// Keep mutations blocked until shutdown once the installer is launched.
		defer func() {
			if !s.installing {
				s.operations.Unlock()
			}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := s.updater.DownloadAndInstall(ctx); err != nil {
		return fmt.Errorf("could not prepare update: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	_ = os.Remove(updateErrorPath())
	if err := s.installer(ctx, s.updater.DownloadedPath(), executable); err != nil {
		return fmt.Errorf("could not install update: %w", err)
	}
	s.installing = true
	// Let the binding response reach the UI before shutting down the webview.
	time.AfterFunc(time.Second, s.quit)
	return nil
}

func truncate(text string, max int) string {
	if len(text) > max {
		return text[:max] + "…"
	}
	return text
}

func updateErrorPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "depmanager-update-error.txt")
}
