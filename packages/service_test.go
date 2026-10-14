package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestPnpmSelfUpdate(t *testing.T) {
	for _, packageName := range []string{"pnpm", "@pnpm/exe"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%t", packageName, fail), func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"packageManager":"pnpm@11.5.2"}`), 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Setenv("DEPMANAGER_TEST_PNPM_FAIL", fmt.Sprint(fail))
				name := "pnpm"
				script := `#!/bin/sh
case "$1" in
  view)
    [ "$2" = 'pnpm@latest' ] || exit 3
    printf '"99.0.0"\n'
    ;;
  self-update)
    [ "$#" -eq 1 ] || exit 4
    pwd
    if [ "$DEPMANAGER_TEST_PNPM_FAIL" = 'true' ]; then
      printf 'test self-update failure\n' >&2
      exit 7
    fi
    ;;
  *) exit 2 ;;
esac
`
				if runtime.GOOS == "windows" {
					name = "pnpm.cmd"
					script = "@echo off\r\n" +
						"if \"%~1\"==\"view\" goto view\r\n" +
						"if \"%~1\"==\"self-update\" goto self\r\nexit /b 2\r\n:view\r\n" +
						"if not \"%~2\"==\"pnpm@latest\" exit /b 3\r\necho \"99.0.0\"\r\nexit /b 0\r\n:self\r\n" +
						"if not \"%~2\"==\"\" exit /b 4\r\necho %CD%\r\n" +
						"if \"%DEPMANAGER_TEST_PNPM_FAIL%\"==\"true\" goto fail\r\nexit /b 0\r\n:fail\r\n" +
						"echo test self-update failure 1>&2\r\nexit /b 7\r\n"
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				latestVersionCache.Delete("pnpm:pnpm")
				t.Cleanup(func() {
					latestVersionCache.Delete("pnpm:pnpm")
					latestVersionCache.Delete("pnpm:@pnpm/exe")
				})
				service := &PackageService{}
				if version := service.GetLatestPackageVersion("pnpm", packageName); version != "99.0.0" {
					t.Fatalf("self-update checked the wrong release: %q", version)
				}
				result := service.RunPackageAction("pnpm", packageName, "update")
				if result.Success == fail {
					t.Fatalf("unexpected self-update result: %+v", result)
				}
				workingDir := strings.TrimSpace(strings.Split(result.Output, "\n")[0])
				if workingDir == dir || !strings.HasPrefix(filepath.Base(workingDir), "depmanager-pnpm-self-update-") {
					t.Fatalf("self-update did not run outside the pinned project: %+v", result)
				}
				if _, err := os.Stat(workingDir); !os.IsNotExist(err) {
					t.Fatalf("self-update did not clean up its working directory: %v", err)
				}
				if fail && !strings.Contains(result.Output, "test self-update failure") {
					t.Fatalf("self-update failure output was lost: %+v", result)
				}
				if !fail {
					if !strings.Contains(result.Message, "pnpm self-update") {
						t.Fatalf("self-update was not identified: %+v", result)
					}
					if _, cached := latestVersionCache.Load("pnpm:pnpm"); cached {
						t.Fatal("self-update left the old release cached")
					}
				}
			})
		}
	}
}

func TestPnpmSelfUpdateExcludesOtherPnpmActions(t *testing.T) {
	service := &PackageService{}
	service.pnpmOperations.RLock()
	result := service.RunPackageAction("pnpm", "@pnpm/exe", "update")
	service.pnpmOperations.RUnlock()
	if result.Success || !strings.Contains(result.Message, "current package operations") {
		t.Fatalf("self-update overlapped a package operation: %+v", result)
	}
	service.pnpmOperations.Lock()
	result = service.RunPackageAction("pnpm", "example", "update")
	service.pnpmOperations.Unlock()
	if result.Success || !strings.Contains(result.Message, "self-update") {
		t.Fatalf("a package operation overlapped self-update: %+v", result)
	}
}

func TestPackageManagerQueriesOverlap(t *testing.T) {
	if dir := os.Getenv("DEPMANAGER_TEST_DISCOVERY"); dir != "" {
		var args []string
		for i, arg := range os.Args {
			if arg == "--" {
				args = os.Args[i+1:]
				break
			}
		}
		if len(args) < 2 {
			os.Exit(2)
		}
		manager, operation := args[0], args[1]
		if manager != "bun" {
			// Neither query succeeds until its sibling starts. Serial queries
			// fail this handshake instead of merely making the test slower.
			if err := os.WriteFile(filepath.Join(dir, manager+operation), nil, 0600); err != nil {
				os.Exit(3)
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				_, versionErr := os.Stat(filepath.Join(dir, manager+"--version"))
				_, listErr := os.Stat(filepath.Join(dir, manager+"ls"))
				if versionErr == nil && listErr == nil {
					break
				}
				if time.Now().After(deadline) {
					os.Exit(4)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
		if operation == "--version" {
			fmt.Println("1.2.3")
		} else if operation == "ls" {
			fmt.Println(`{"dependencies":{"example":{"version":"4.5.6"}}}`)
		} else {
			os.Exit(5)
		}
		os.Exit(0)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DEPMANAGER_TEST_DISCOVERY", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BUN_INSTALL", dir)
	globalDir := filepath.Join(dir, "install", "global")
	if err := os.MkdirAll(globalDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "package.json"), []byte(`{"dependencies":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, manager := range packageManagerNames {
		name := manager
		script := "#!/bin/sh\nexec '" + strings.ReplaceAll(os.Args[0], "'", "'\"'\"'") + "' -test.run=^TestPackageManagerQueriesOverlap$ -- " + manager + " \"$@\"\n"
		if runtime.GOOS == "windows" {
			name += ".cmd"
			script = "@echo off\r\n\"" + os.Args[0] + "\" -test.run=^TestPackageManagerQueriesOverlap$ -- " + manager + " %*\r\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	managers := (&PackageService{}).GetPackageManagers()
	for i, manager := range managers {
		if manager.Name != packageManagerNames[i] || !manager.Installed || manager.Version != "1.2.3" || manager.Error != "" {
			t.Fatalf("concurrent discovery failed: %+v", manager)
		}
		if manager.Name != "bun" && (len(manager.Packages) != 1 || manager.Packages[0].Version != "4.5.6") {
			t.Fatalf("concurrent discovery lost packages: %+v", manager)
		}
	}
}
