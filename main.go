package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()
	ws := app.Settings.Get().Window

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "ZP Redis Manager",
		Width:     ws.Width,
		Height:    ws.Height,
		MinWidth:  1100,
		MinHeight: 700,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 17, B: 23, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.onShutdown,
		Bind: []interface{}{
			app,
			app.Connection,
			app.Key,
			app.Value,
			app.Server,
			app.Cli,
			app.Transfer,
			app.Ts,
			app.Bf,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
