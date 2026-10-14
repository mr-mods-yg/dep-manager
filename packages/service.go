package packages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GlobalPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type PackageManagerInfo struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Installed bool            `json:"installed"`
	Packages  []GlobalPackage `json:"packages"`
	Error     string          `json:"error,omitempty"`
}

type PackageActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

type PackageService struct {
	operations     *sync.RWMutex
	pnpmOperations sync.RWMutex
	emitProgress   func(PackageActionProgress)
}

func New(operations *sync.RWMutex) *PackageService {
	return &PackageService{operations: operations}
}

func Configure(s *PackageService, emitProgress func(PackageActionProgress)) {
	s.emitProgress = emitProgress
}

type latestVersionCacheEntry struct {
	version   string
	expiresAt time.Time
}

var latestVersionCache sync.Map

var validPackageName = regexp.MustCompile(`(?i)^(?:@[a-z0-9][a-z0-9._~-]*/)?[a-z0-9][a-z0-9._~-]*$`)

var packageManagerNames = []string{"npm", "pnpm", "bun"}

// GetPackageManagers discovers the supported package managers and their direct global installs.
func (s *PackageService) GetPackageManagers() []PackageManagerInfo {
	managers := make([]PackageManagerInfo, len(packageManagerNames))
	var wg sync.WaitGroup
	for i, name := range packageManagerNames {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			manager := PackageManagerInfo{Name: name, Packages: []GlobalPackage{}}
			if _, err := exec.LookPath(name); err != nil {
				managers[i] = manager
				return
			}

			manager.Installed = true
			var queries sync.WaitGroup
			queries.Add(1)
			go func() {
				defer queries.Done()
				version, _, err := runPackageManager(name, 8*time.Second, "--version")
				if err == nil {
					manager.Version = strings.TrimSpace(string(version))
				}
			}()

			packages, listErr := listGlobalPackages(name)
			manager.Packages = packages
			if listErr != nil {
				manager.Error = listErr.Error()
			}
			queries.Wait()
			managers[i] = manager
		}(i, name)
	}
	wg.Wait()
	return managers
}

// GetLatestPackageVersion returns the registry's latest-tag version for a package.
// An empty result means the registry could not be checked.
func (s *PackageService) GetLatestPackageVersion(manager, packageName string) string {
	manager = strings.ToLower(strings.TrimSpace(manager))
	packageName = strings.TrimSpace(packageName)
	if !isSupportedManager(manager) || !isValidPackageName(packageName) {
		return ""
	}
	// Self-update follows pnpm's release, including standalone @pnpm/exe
	// installations that migrate to the pnpm package in newer releases.
	if isPnpmSelfUpdate(manager, packageName) {
		packageName = "pnpm"
	}

	cacheKey := manager + ":" + packageName
	if value, ok := latestVersionCache.Load(cacheKey); ok {
		entry := value.(latestVersionCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.version
		}
		latestVersionCache.Delete(cacheKey)
	}

	version, _, err := runPackageManager(manager, 10*time.Second, latestVersionArgs(manager, packageName)...)
	if err != nil {
		return ""
	}
	latestVersion := parseLatestVersionOutput(version)
	if latestVersion == "" {
		return ""
	}
	latestVersionCache.Store(cacheKey, latestVersionCacheEntry{
		version:   latestVersion,
		expiresAt: time.Now().Add(5 * time.Minute),
	})
	return latestVersion
}

// RunPackageAction updates a global package to its latest version or removes it.
func (s *PackageService) RunPackageAction(manager, packageName, action string) PackageActionResult {
	if s.operations != nil {
		if !s.operations.TryRLock() {
			return PackageActionResult{Message: "DepManager is updating. Wait for the application to restart."}
		}
		defer s.operations.RUnlock()
	}
	manager = strings.ToLower(strings.TrimSpace(manager))
	packageName = strings.TrimSpace(packageName)
	if !isValidPackageName(packageName) {
		return PackageActionResult{Message: "That package name is not valid."}
	}
	if action != "update" && action != "remove" {
		return PackageActionResult{Message: "Choose update or remove."}
	}
	if !isSupportedManager(manager) {
		return PackageActionResult{Message: "That package manager is not supported."}
	}
	selfUpdate := action == "update" && isPnpmSelfUpdate(manager, packageName)
	if manager == "pnpm" {
		if selfUpdate {
			if !s.pnpmOperations.TryLock() {
				return PackageActionResult{Message: "Wait for pnpm's current package operations to finish before updating pnpm."}
			}
			defer s.pnpmOperations.Unlock()
		} else {
			if !s.pnpmOperations.TryRLock() {
				return PackageActionResult{Message: "pnpm is updating. Wait for its self-update to finish."}
			}
			defer s.pnpmOperations.RUnlock()
		}
	}
	progress := func(stage string) {
		if s.emitProgress != nil {
			s.emitProgress(PackageActionProgress{Manager: manager, PackageName: packageName, Action: action, Stage: stage})
		}
	}
	progress("Preparing")
	if _, err := exec.LookPath(manager); err != nil {
		return PackageActionResult{Message: manager + " is no longer available on PATH."}
	}

	args := packageActionArgs(manager, packageName, action)
	workingDir := ""
	if selfUpdate {
		// In a project with a packageManager pin, self-update changes that pin
		// instead of the global CLI. Use an empty directory for the global update.
		var err error
		workingDir, err = os.MkdirTemp("", "depmanager-pnpm-self-update-")
		if err != nil {
			return PackageActionResult{Message: "Could not prepare pnpm self-update: " + err.Error()}
		}
		defer os.RemoveAll(workingDir)
	}
	stage := "Updating"
	if selfUpdate {
		stage = "Updating pnpm"
	} else if action == "remove" {
		stage = "Removing"
	} else if manager == "npm" {
		stage = "Installing"
	}
	progress(stage)
	// Ask for readable progress even when the CLI is connected to a pipe.
	if manager == "npm" {
		args = append(args, "--loglevel=http")
	} else if manager == "pnpm" && !selfUpdate {
		args = append(args, "--reporter=append-only")
	}
	output, stderr, err := runPackageManagerInDir(manager, 10*time.Minute, progress, workingDir, args...)
	combined := strings.TrimSpace(string(output) + "\n" + string(stderr))
	if err != nil {
		message := fmt.Sprintf("Could not %s %s with %s: %s", action, packageName, manager, err)
		return PackageActionResult{Message: message, Output: truncate(combined, 6000)}
	}
	latestVersionCache.Delete(manager + ":" + packageName)
	if selfUpdate {
		latestVersionCache.Delete("pnpm:pnpm")
		latestVersionCache.Delete("pnpm:@pnpm/exe")
		return PackageActionResult{Success: true, Message: "Updated the pnpm package manager with pnpm self-update.", Output: truncate(combined, 6000)}
	}

	verb := "Updated"
	if action == "remove" {
		verb = "Removed"
	}
	message := fmt.Sprintf("%s %s with %s.", verb, packageName, manager)
	if action == "update" && manager == "bun" && strings.Contains(combined, "(no changes)") {
		message = fmt.Sprintf("No changes to %s with %s.", packageName, manager)
	}
	return PackageActionResult{Success: true, Message: message, Output: truncate(combined, 6000)}
}

func isSupportedManager(manager string) bool {
	for _, supported := range packageManagerNames {
		if manager == supported {
			return true
		}
	}
	return false
}

func isValidPackageName(name string) bool {
	if !validPackageName.MatchString(name) {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(name, "@"), "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func isPnpmSelfUpdate(manager, packageName string) bool {
	return manager == "pnpm" && (packageName == "pnpm" || packageName == "@pnpm/exe")
}

func packageActionArgs(manager, packageName, action string) []string {
	switch manager {
	case "npm":
		if action == "update" {
			return []string{"install", "--global", packageName + "@latest"}
		}
		return []string{"uninstall", "--global", packageName}
	case "pnpm":
		if action == "update" {
			if isPnpmSelfUpdate(manager, packageName) {
				return []string{"self-update"}
			}
			return []string{"update", "--global", "--latest", packageName}
		}
		return []string{"remove", "--global", packageName}
	case "bun":
		if action == "update" {
			return []string{"update", "--global", "--latest", packageName}
		}
		return []string{"remove", "--global", packageName}
	default:
		return nil
	}
}

func latestVersionArgs(manager, packageName string) []string {
	spec := packageName + "@latest"
	switch manager {
	case "npm", "pnpm":
		return []string{"view", spec, "version", "--json"}
	case "bun":
		// Without --global, Bun requires a package.json in the app's working directory.
		return []string{"pm", "--global", "view", spec, "version", "--json"}
	default:
		return nil
	}
}

func parseLatestVersionOutput(data []byte) string {
	var version string
	if err := json.Unmarshal(data, &version); err == nil {
		return strings.TrimSpace(version)
	}

	var versions []string
	if err := json.Unmarshal(data, &versions); err == nil && len(versions) > 0 {
		return strings.TrimSpace(versions[0])
	}

	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err == nil && manifest.Version != "" {
		return strings.TrimSpace(manifest.Version)
	}

	return strings.Trim(strings.TrimSpace(string(data)), "\"'")
}

func listGlobalPackages(manager string) ([]GlobalPackage, error) {
	switch manager {
	case "npm":
		stdout, stderr, err := runPackageManager(manager, 30*time.Second, "ls", "--global", "--depth=0", "--json")
		packages, parseErr := parseJSONGlobalPackages(stdout)
		if parseErr == nil {
			fillMissingPackageVersions(manager, packages)
			return packages, nil // npm can return exit status 1 and valid JSON for extraneous packages.
		}
		return nil, commandOrParseError(err, stderr, parseErr)
	case "pnpm":
		stdout, stderr, err := runPackageManager(manager, 30*time.Second, "ls", "--global", "--depth=0", "--json")
		packages, parseErr := parseJSONGlobalPackages(stdout)
		if parseErr == nil {
			fillMissingPackageVersions(manager, packages)
			return packages, nil
		}
		return nil, commandOrParseError(err, stderr, parseErr)
	case "bun":
		return listBunGlobalPackages()
	default:
		return nil, errors.New("unsupported package manager")
	}
}

// Some CLI package lists omit versions for linked or incomplete installs.
// Only a local installed manifest can supply the actual installed version.
func fillMissingPackageVersions(manager string, packages []GlobalPackage) {
	for _, pkg := range packages {
		if strings.TrimSpace(pkg.Version) == "" {
			stdout, _, err := runPackageManager(manager, 8*time.Second, "root", "--global")
			if err == nil {
				fillPackageVersionsFromRoot(packages, strings.TrimSpace(string(stdout)))
			}
			return
		}
	}
}

func fillPackageVersionsFromRoot(packages []GlobalPackage, root string) {
	if !filepath.IsAbs(root) {
		return
	}
	for i := range packages {
		if strings.TrimSpace(packages[i].Version) != "" || !isValidPackageName(packages[i].Name) {
			continue
		}
		if installed, ok := readPackageManifest(filepath.Join(root, filepath.FromSlash(packages[i].Name), "package.json")); ok {
			packages[i].Version = installed.Version
		}
	}
}

func commandOrParseError(commandErr error, stderr []byte, parseErr error) error {
	if commandErr != nil {
		message := commandErr.Error()
		if details := strings.TrimSpace(string(stderr)); details != "" {
			message += ": " + truncate(details, 500)
		}
		return errors.New(message)
	}
	return parseErr
}

type packageTree struct {
	Name         string                     `json:"name"`
	Version      string                     `json:"version"`
	Dependencies map[string]json.RawMessage `json:"dependencies"`
}

func parseJSONGlobalPackages(data []byte) ([]GlobalPackage, error) {
	var trees []packageTree
	if err := json.Unmarshal(data, &trees); err == nil {
		if len(trees) == 0 {
			return []GlobalPackage{}, nil
		}
		return packagesFromDependencies(trees[0].Dependencies), nil
	}

	var tree packageTree
	if err := json.Unmarshal(data, &tree); err == nil && tree.Dependencies != nil {
		return packagesFromDependencies(tree.Dependencies), nil
	}
	return nil, errors.New("could not read the package manager's JSON package list")
}

func packagesFromDependencies(dependencies map[string]json.RawMessage) []GlobalPackage {
	packages := make([]GlobalPackage, 0, len(dependencies))
	for name, raw := range dependencies {
		if !isValidPackageName(name) {
			continue
		}
		var item struct {
			Version string `json:"version"`
		}
		_ = json.Unmarshal(raw, &item)
		packages = append(packages, GlobalPackage{Name: name, Version: item.Version})
	}
	return sortedPackages(packages)
}

func listBunGlobalPackages() ([]GlobalPackage, error) {
	var firstErr error
	for _, globalDir := range bunGlobalDirs() {
		packages, err := packagesFromBunGlobalDir(globalDir)
		if err == nil {
			return packages, nil
		}
		if !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return []GlobalPackage{}, nil
}

func bunGlobalDirs() []string {
	dirs := make([]string, 0, 5)
	if install := os.Getenv("BUN_INSTALL"); install != "" {
		dirs = append(dirs, filepath.Join(install, "install", "global"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".bun", "install", "global"))
		for _, config := range []string{filepath.Join(home, "bunfig.toml"), filepath.Join(home, ".bunfig.toml")} {
			if globalDir := configuredBunGlobalDir(config); globalDir != "" {
				dirs = append(dirs, globalDir)
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if globalDir := configuredBunGlobalDir(filepath.Join(cwd, "bunfig.toml")); globalDir != "" {
			dirs = append(dirs, globalDir)
		}
	}

	seen := make(map[string]bool, len(dirs))
	unique := dirs[:0]
	for _, dir := range dirs {
		key := strings.ToLower(filepath.Clean(dir))
		if !seen[key] {
			seen[key] = true
			unique = append(unique, dir)
		}
	}
	return unique
}

func packagesFromBunGlobalDir(globalDir string) ([]GlobalPackage, error) {
	manifestPath := filepath.Join(globalDir, "package.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("could not read Bun global package manifest: %w", err)
	}

	packages := make([]GlobalPackage, 0, len(manifest.Dependencies))
	for name := range manifest.Dependencies {
		if !isValidPackageName(name) {
			continue
		}
		pkg, ok := readPackageManifest(filepath.Join(globalDir, "node_modules", filepath.FromSlash(name), "package.json"))
		if !ok {
			continue
		}
		// Use the dependency key so npm aliases remain uninstallable by their
		// Bun global name, while taking the actual installed version from disk.
		pkg.Name = name
		packages = append(packages, pkg)
	}
	return sortedPackages(packages), nil
}

func configuredBunGlobalDir(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	insideInstall := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			insideInstall = strings.TrimSpace(line[1:len(line)-1]) == "install"
			continue
		}
		if !insideInstall {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "globalDir" {
			continue
		}
		value = strings.TrimSpace(value)
		if decoded, err := strconv.Unquote(value); err == nil {
			value = decoded
		} else {
			value = strings.Trim(value, "\"'")
		}
		value = os.ExpandEnv(value)
		if strings.HasPrefix(value, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				value = filepath.Join(home, strings.TrimLeft(value[1:], `/\`))
			}
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(configPath), value)
		}
		return filepath.Clean(value)
	}
	return ""
}

func packagesFromManifest(nodeModules string) ([]GlobalPackage, error) {
	entries, err := os.ReadDir(nodeModules)
	if err != nil {
		return nil, err
	}
	packages := make([]GlobalPackage, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		entryPath := filepath.Join(nodeModules, entry.Name())
		if strings.HasPrefix(entry.Name(), "@") {
			scopeEntries, err := os.ReadDir(entryPath)
			if err != nil {
				continue
			}
			for _, scoped := range scopeEntries {
				if pkg, ok := readPackageManifest(filepath.Join(entryPath, scoped.Name(), "package.json")); ok {
					packages = append(packages, pkg)
				}
			}
			continue
		}
		if pkg, ok := readPackageManifest(filepath.Join(entryPath, "package.json")); ok {
			packages = append(packages, pkg)
		}
	}
	return sortedPackages(packages), nil
}

func readPackageManifest(path string) (GlobalPackage, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GlobalPackage{}, false
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &manifest) != nil || !isValidPackageName(manifest.Name) {
		return GlobalPackage{}, false
	}
	return GlobalPackage{Name: manifest.Name, Version: manifest.Version}, true
}

func sortedPackages(packages []GlobalPackage) []GlobalPackage {
	sort.Slice(packages, func(i, j int) bool {
		return strings.ToLower(packages[i].Name) < strings.ToLower(packages[j].Name)
	})
	return packages
}

func runPackageManager(name string, timeout time.Duration, args ...string) ([]byte, []byte, error) {
	return runPackageManagerWithProgress(name, timeout, nil, args...)
}

func runPackageManagerWithProgress(name string, timeout time.Duration, progress func(string), args ...string) ([]byte, []byte, error) {
	return runPackageManagerInDir(name, timeout, progress, "", args...)
}

func runPackageManagerInDir(name string, timeout time.Duration, progress func(string), dir string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		// The npm ecosystem commonly installs .cmd shims, which Windows cannot
		// execute directly. Package names are validated before reaching this path.
		parts := append([]string{name}, args...)
		command = exec.CommandContext(ctx, "cmd.exe", append([]string{"/d", "/s", "/c", strings.Join(parts, " ")})...)
	} else {
		command = exec.CommandContext(ctx, name, args...)
	}
	command.Dir = dir
	configurePackageCommand(command)
	var stdout, stderr bytes.Buffer
	if progress == nil {
		command.Stdout = &stdout
		command.Stderr = &stderr
	} else {
		reporter := &packageProgressReporter{emit: progress}
		out := &packageProgressWriter{output: &stdout, reporter: reporter}
		errOut := &packageProgressWriter{output: &stderr, reporter: reporter}
		command.Stdout = out
		command.Stderr = errOut
		defer out.flush()
		defer errOut.flush()
	}
	err := command.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func truncate(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "…"
}
