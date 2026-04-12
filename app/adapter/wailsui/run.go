package wailsui

import (
	"io/fs"

	"app/config"
	"app/service"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// Run starts the Wails GUI with the given frontend asset subtree (e.g. frontend/dist).
func Run(cfg *config.Config, assets fs.FS) error {
	rt := service.NewRuntime(cfg)
	rt.SetEventSink(NewEventSink())

	return wails.Run(&options.App{
		Title:  "SecureP2P",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        rt.Startup,
		OnDomReady:       rt.DomReady,
		OnBeforeClose:    rt.BeforeClose,
		OnShutdown:       rt.Shutdown,
		Bind: []interface{}{
			rt,
		},
	})
}
