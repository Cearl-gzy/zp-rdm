package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"zp-rdm/internal/redisx"
	"zp-rdm/internal/settings"
	"zp-rdm/internal/store"
	"zp-rdm/service"
)

// AppVersion 应用版本号（打包时同步 wails.json info.productVersion）
const AppVersion = "1.1.0"

// updateManifestURL 更新清单地址（发布服务器上的 JSON：{"version":"x.y.z","url":"下载地址"}）
const updateManifestURL = ""

// App struct
type App struct {
	ctx context.Context

	Store    *store.Store
	Manager  *redisx.Manager
	Settings *settings.Manager

	Connection *service.ConnectionService
	Key        *service.KeyService
	Value      *service.ValueService
	Server     *service.ServerService
	Cli        *service.CliService
	Transfer   *service.TransferService
}

// NewApp creates a new App application struct
func NewApp() *App {
	st, err := store.New()
	if err != nil {
		panic(err)
	}
	sm, err := settings.New()
	if err != nil {
		panic(err)
	}
	m := redisx.NewManager()
	return &App{
		Store:      st,
		Manager:    m,
		Settings:   sm,
		Connection: service.NewConnectionService(st, m),
		Key:        service.NewKeyService(m),
		Value:      service.NewValueService(m),
		Server:     service.NewServerService(m),
		Cli:        service.NewCliService(m),
		Transfer:   service.NewTransferService(m),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 恢复窗口位置 / 最大化状态
	if ws := a.Settings.Get().Window; ws.X >= 0 && ws.Y >= 0 {
		wailsRuntime.WindowSetPosition(ctx, ws.X, ws.Y)
		if ws.Maximised {
			wailsRuntime.WindowMaximise(ctx)
		}
	}
	a.Connection.Startup(ctx)
	a.Key.Startup(ctx)
	a.Value.Startup(ctx)
	a.Server.Startup(ctx)
	a.Cli.Startup(ctx)
	a.Transfer.Startup(ctx)
}

// onShutdown 退出前保存窗口几何状态
func (a *App) onShutdown(ctx context.Context) {
	prev := a.Settings.Get().Window
	maximised := wailsRuntime.WindowIsMaximised(ctx)
	w, h := wailsRuntime.WindowGetSize(ctx)
	x, y := wailsRuntime.WindowGetPosition(ctx)
	ws := settings.WindowState{
		Width:     prev.Width,
		Height:    prev.Height,
		X:         x,
		Y:         y,
		Maximised: maximised,
	}
	// 最大化/最小化时不覆盖上次正常窗口尺寸
	if !maximised && w > 0 && h > 0 {
		ws.Width = w
		ws.Height = h
	}
	_ = a.Settings.SaveWindowState(ws)
}

// ---------- 设置绑定 ----------

// GetSettings 读取应用设置
func (a *App) GetSettings() settings.Settings {
	return a.Settings.Get()
}

// SaveCliHistory 保存 CLI 命令历史
func (a *App) SaveCliHistory(history []string) error {
	return a.Settings.SaveCliHistory(history)
}

// SaveFrontendState 保存前端状态（主题/侧栏宽度/每连接 db）
func (a *App) SaveFrontendState(theme string, sidebarWidth int, lastDb map[string]int) error {
	return a.Settings.SaveFrontendState(theme, sidebarWidth, lastDb)
}

// Version 应用版本号
func (a *App) Version() string { return AppVersion }

// CheckUpdate 检查更新（需要配置 updateManifestURL）
func (a *App) CheckUpdate() (string, error) {
	if updateManifestURL == "" {
		return "", fmt.Errorf("未配置更新源")
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(updateManifestURL) //nolint:gosec 地址为编译期常量
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var manifest struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("更新清单格式错误")
	}
	if manifest.Version == "" || manifest.Version == AppVersion {
		return "", nil // 已是最新
	}
	if !strings.HasPrefix(manifest.URL, "http") {
		return "", fmt.Errorf("更新清单缺少下载地址")
	}
	return manifest.URL, nil
}

// PickOpenFile 打开文件选择对话框
func (a *App) PickOpenFile(title string) (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:   title,
		Filters: []wailsRuntime.FileFilter{{DisplayName: "JSON 文件", Pattern: "*.json"}},
	})
}

// PickSaveFile 保存文件对话框
func (a *App) PickSaveFile(title, defaultFilename string) (string, error) {
	return wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           title,
		DefaultFilename: defaultFilename,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "数据文件", Pattern: "*.json;*.jsonl;*.csv"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
}
