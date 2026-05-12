package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/NablaShell/LastChance/internal/network"
	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist .env
var assets embed.FS

// LoadConfig правильно инициализирует настройки после загрузки окружения
func LoadConfig() *network.Config {
	return &network.Config{
		MaskHeaderName:  getEnv("LC_MASK_HEADER_NAME", "X-DNS-Connect"),
		MaskHeaderValue: getEnv("LC_MASK_HEADER_VALUE", "We_Are_Not_Legion"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func initEnv() {
	// 1. Пытаемся загрузить локальный .env (приоритет выше, чем у вшитого)
	if err := godotenv.Load(); err == nil {
		log.Println("[INIT] Local .env loaded")
	}

	// 2. Дозагружаем вшитый .env для переменных, которых нет в локальном
	envData, err := assets.ReadFile(".env")
	if err == nil {
		myEnv, parseErr := godotenv.Unmarshal(string(envData))
		if parseErr == nil {
			for key, value := range myEnv {
				// Устанавливаем только если переменная еще не задана
				if _, exists := os.LookupEnv(key); !exists {
					os.Setenv(key, value)
				}
			}
		}
	}
}

func main() {
	// Инициализируем окружение ДО создания приложения
	initEnv()

	// Загружаем конфиг, когда переменные точно в os.Environ
	cfg := LoadConfig()

	// Создаем экземпляр приложения, прокидываем конфиг в NewApp
	app := NewApp(cfg) 

	err := wails.Run(&options.App{
		Title:  getEnv("APP_TITLE", "Not LastChance"), // Стелс-заголовок из ENV
		Width:  1024,
		Height: 768,
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
