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
	LatestVersion    string `json:"latestVersion,omitempty"`
	Available        bool   `json:"available"`
	CanInstall       bool   `json:"canInstall"`
	Message          string `json:"message"`
	ReleaseURL       string `json:"releaseURL"`
	LastError        string `json:"lastError,omitempty"`
	DevelopmentBuild bool   `json:"developmentBuild"`
}

type Service struct {
	version    string
	canInstall bool
	operations *sync.RWMutex
	installer  func(context.Context, string, string) error
	updater    *updater.Updater
	quit       func()
	mu         sync.Mutex
	installing bool
}

func New(version string, operations *sync.RWMutex) *Service {
	return &Service{
		version:    version,
		canInstall: appUpdatesEnabled && (runtime.GOOS == "windows" || runtime.GOOS == "linux") && runtime.GOARCH == "amd64",
		operations: operations,
		installer:  installAppUpdate,
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

func Configure(s *Service, runtimeUpdater *updater.Updater, quit func()) error {
	provider, err := githubprovider.New(githubprovider.Config{
		Repository:    releaseRepository,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  matchUpdateAsset,
	})
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
		CanInstall:       s.canInstall,
		DevelopmentBuild: !appUpdatesEnabled,
		Message:          "Checks GitHub for the latest stable release.",
		ReleaseURL:       "https://github.com/" + releaseRepository + "/releases/latest",
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

func (s *Service) CheckForUpdates() (AppUpdateInfo, error) {
	info := s.GetAppInfo()
	if !s.mu.TryLock() {
		return info, errors.New("an update operation is already running")
	}
	defer s.mu.Unlock()
	if s.installing {
		return info, errors.New("the application is restarting to finish an update")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	release, err := s.updater.Check(ctx)
	if err != nil {
		return info, fmt.Errorf("could not check GitHub releases: %w", err)
	}
	info.Message = "You're using the latest available version."
	if release != nil {
		info.LatestVersion = release.Version
		info.Available = true
		info.Message = "DepManager " + release.Version + " is available."
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
