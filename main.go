// main.go
package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/NablaShell/LastChance/internal/network"
	"github.com/NablaShell/LastChance/internal/storage"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func initEnv() {
	// --- Исправление ошибки Signal 11 (конфликт сигналов WebKit и Go) ---
	// Эти переменные ДОЛЖНЫ быть установлены до запуска графического движка
	os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	os.Setenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS", "1")
}

func main() {
	// Устанавливаем переменные окружения до любого GUI-кода
	initEnv()

	// Определяем базовую директорию (портабельная или ~/.local/share/lastchance)
	baseDir, err := storage.GetBaseDir()
	if err != nil {
		log.Fatalf("Failed to determine base directory: %v", err)
	}
	log.Printf("Base directory: %s", baseDir)

	// Загружаем конфигурацию из global.conf в базовой директории
	cfg, err := network.LoadConfig(baseDir)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Создаём приложение, передавая конфиг и базовую директорию
	app := NewApp(cfg, baseDir)

	// Заголовок окна можно брать из конфига или оставить по умолчанию
	appTitle := "Not LastChance"
	if cfg.MaskHeaderName != "" {
		appTitle = cfg.MaskHeaderName // или отдельное поле, если потребуется
	}

	err = wails.Run(&options.App{
		Title:     appTitle,
		Width:     1024,
		Height:    768,
		MinWidth:  800,
		MinHeight: 600,

		Frameless:       true,
		CSSDragProperty: "--wails-drag",
		CSSDragValue:    "drag",

		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 15, A: 255},

		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
		},

		OnShutdown: app.shutdown,

		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal("Application error:", err)
	}
}
