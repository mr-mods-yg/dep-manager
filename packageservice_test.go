package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestMissingInstalledVersionsUseLocalManifest(t *testing.T) {
	root := t.TempDir()
	for name, manifest := range map[string]string{
		"@deepseek-ai/dsh": `{"name":"@deepseek-ai/dsh","version":"0.2.0-rc.2"}`,
		"linked-alias":     `{"name":"actual-package","version":"2.4.1"}`,
		"already-known":    `{"name":"already-known","version":"9.0.0"}`,
	} {
		dir := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	packages, err := parseJSONGlobalPackages([]byte(`{"dependencies":{
		"@deepseek-ai/dsh":{"overridden":false},
		"linked-alias":{},"already-known":{"version":"1.0.0"},"broken-install":{}
	}}`))
	if err != nil {
		t.Fatal(err)
	}
	fillPackageVersionsFromRoot(packages, root)
	want := map[string]string{"@deepseek-ai/dsh": "0.2.0-rc.2", "linked-alias": "2.4.1", "already-known": "1.0.0", "broken-install": ""}
	for _, pkg := range packages {
		if pkg.Version != want[pkg.Name] {
			t.Errorf("%s version = %q, want %q", pkg.Name, pkg.Version, want[pkg.Name])
		}
	}
	if len(packages) != len(want) {
		t.Fatalf("got %d packages, want %d", len(packages), len(want))
	}
}

func TestPackageMutationsBlockedDuringAppUpdate(t *testing.T) {
	service := &PackageService{operations: new(sync.RWMutex)}
	service.operations.Lock()
	defer service.operations.Unlock()
	result := service.RunPackageAction("npm", "example", "update")
	if result.Success || !strings.Contains(result.Message, "updating") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestBunVersionCheckWithoutLocalManifestAndNoChangeUpdate(t *testing.T) {
	// Reproduce a packaged app running outside a JavaScript project. The fake
	// CLI refuses local metadata queries, as Bun does without package.json.
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	name := "bun"
	script := `#!/bin/sh
case "$1" in
  pm)
    [ "$2" = "--global" ] || exit 1
    printf '"2.0.28"\n'
    ;;
  update)
    printf 'bun update v1.4.3\nChecked 3 installs across 14 packages (no changes)\n'
    ;;
  *) exit 1 ;;
esac
`
	if runtime.GOOS == "windows" {
		name = "bun.cmd"
		script = "@echo off\r\n" +
			"if \"%1\"==\"pm\" goto view\r\n" +
			"if \"%1\"==\"update\" goto update\r\n" +
			"exit /b 1\r\n:view\r\n" +
			"if not \"%2\"==\"--global\" exit /b 1\r\n" +
			"echo \"2.0.28\"\r\nexit /b 0\r\n:update\r\n" +
			"echo bun update v1.4.3\r\n" +
			"echo Checked 3 installs across 14 packages (no changes)\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	service := &PackageService{}
	const pkg = "@depmanager-test/version-check"
	const cacheKey = "bun:" + pkg
	t.Cleanup(func() { latestVersionCache.Delete(cacheKey) })
	if version := service.GetLatestPackageVersion("bun", pkg); version != "2.0.28" {
		t.Fatalf("latest version without local manifest = %q", version)
	}
	result := service.RunPackageAction("bun", pkg, "update")
	if !result.Success || result.Message != "No changes to "+pkg+" with bun." {
		t.Fatalf("unexpected no-change result: %+v", result)
	}
	if strings.Contains(result.Message, "bun update v1.4.3") || !strings.Contains(result.Output, "bun update v1.4.3") {
		t.Fatalf("Bun version should only appear in command output: %+v", result)
	}
	if _, cached := latestVersionCache.Load(cacheKey); cached {
		t.Fatal("successful action left a stale registry version in the cache")
	}
}
