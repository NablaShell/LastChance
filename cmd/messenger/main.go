package main

import (
	"context"
	"embed"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist .env
var assets embed.FS

// ============================================================
// ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ МАСКИРОВКИ
// ============================================================

var (
	MaskHeaderName  = getEnv("LC_MASK_HEADER_NAME", "X-DNS-Cookie")
	MaskHeaderValue = getEnv("LC_MASK_HEADER_VALUE", "37deef852bdebedbcf7a279d89e27b93bc7d3f4696b4cddbac8c24fe117619c0")
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// 1. Load environment variables from embedded .env
	envData, err := assets.ReadFile(".env")
	if err == nil {
		myEnv, parseErr := godotenv.Unmarshal(string(envData))
		if parseErr == nil {
			for key, value := range myEnv {
				log.Printf("Embedded config loaded: %s", key)
				if err := os.Setenv(key, value); err != nil {
					log.Printf("Failed to set env %s: %v", key, err)
				}
			}
		} else {
			log.Printf("Warning: Failed to parse embedded .env: %v", parseErr)
		}
	} else {
		// Fallback to local .env file for development
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: No .env file found: %v", err)
		}
	}

	// Create app instance
	app := NewApp()

	// Run the application with frameless window
	err = wails.Run(&options.App{
		Title:     "LastChance Messenger",
		Width:     1024,
		Height:    768,
		MinWidth:  800,
		MinHeight: 600,

		// Frameless window configuration
		Frameless:       true, // Remove OS title bar
		CSSDragProperty: "--wails-drag",
		CSSDragValue:    "drag",

		// Asset server configuration
		AssetServer: &assetserver.Options{
			Assets: assets,
		},

		// Match your app's gradient background
		BackgroundColour: &options.RGBA{
			R: 10,
			G: 10,
			B: 15,
			A: 255,
		},

		// Window positioning
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
		},

		OnShutdown: app.shutdown,

		// Bind your app methods
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal("Application error:", err)
	}
}
