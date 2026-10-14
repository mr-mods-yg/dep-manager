package main

import (
	"depmanager/packages"
	"depmanager/updates"
	"embed"
	"encoding/json"
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/package.json
var appPackageJSON []byte

func main() {
	var manifest struct {
		Version        string `json:"version"`
		ReleaseChannel string `json:"releaseChannel"`
	}
	if err := json.Unmarshal(appPackageJSON, &manifest); err != nil || manifest.Version == "" {
		log.Fatal("application version is missing from frontend/package.json")
	}
	operations := &sync.RWMutex{}
	packageService := packages.New(operations)
	updateService := updates.New(manifest.Version, operations, manifest.ReleaseChannel)
	app := application.New(application.Options{
		Name:        "DepManager",
		Description: "Manage global JavaScript packages",
		Services: []application.Service{
			application.NewService(packageService),
			application.NewService(updateService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	packages.Configure(packageService, func(progress packages.PackageActionProgress) {
		app.Event.Emit(packages.ProgressEvent, progress)
	})
	if err := updates.Configure(updateService, app.Updater, app.Quit); err != nil {
		log.Fatal(err)
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "DepManager",
		Width:  1220,
		Height: 820,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(245, 246, 248),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
