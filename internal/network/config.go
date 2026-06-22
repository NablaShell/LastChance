// internal/network/config.go
package network

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/NablaShell/LastChance/internal/storage"
)

// Config holds all network‑related configuration, including server URLs.
type Config struct {
	// ServerURL is the base URL for message operations (/push, /pull).
	ServerURL string `json:"server_url"`
	// FileServerURL is the base URL for file operations (/upload, /download).
	FileServerURL string `json:"file_server_url"`
	// MaskHeaderName is the name of the disguise header (e.g., X-Requested-With).
	MaskHeaderName string `json:"mask_header_name"`
	// MaskHeaderValue is the value of the disguise header.
	MaskHeaderValue string `json:"mask_header_value"`
}

// DefaultConfig returns a safe, offline‑ready default configuration.
func DefaultConfig() *Config {
	return &Config{
		ServerURL:       "https://msg.example.com",
		FileServerURL:   "https://files.example.com",
		MaskHeaderName:  "X-DNS-Connect",
		MaskHeaderValue: "We_Are_Not_Legion",
	}
}

// LoadConfig reads the configuration from baseDir/global.conf.
// If the file does not exist, it writes the default config and returns it.
func LoadConfig(baseDir string) (*Config, error) {
	safeFS, err := storage.NewSafeFSOps(baseDir)
	if err != nil {
		return nil, fmt.Errorf("cannot init safe filesystem: %w", err)
	}

	configPath := "global.conf"
	data, err := safeFS.SafeReadFile(configPath)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("invalid config file: %w", err)
		}
		// Заполняем пустые поля значениями по умолчанию
		defaults := DefaultConfig()
		if cfg.ServerURL == "" {
			cfg.ServerURL = defaults.ServerURL
		}
		if cfg.FileServerURL == "" {
			cfg.FileServerURL = defaults.FileServerURL
		}
		if cfg.MaskHeaderName == "" {
			cfg.MaskHeaderName = defaults.MaskHeaderName
		}
		if cfg.MaskHeaderValue == "" {
			cfg.MaskHeaderValue = defaults.MaskHeaderValue
		}
		return &cfg, nil
	}

	// Файла нет – создаём с настройками по умолчанию
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	defaultCfg := DefaultConfig()
	jsonData, err := json.MarshalIndent(defaultCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default config: %w", err)
	}
	if err := safeFS.SafeWriteFile(configPath, jsonData, 0600); err != nil {
		return nil, fmt.Errorf("failed to write default config: %w", err)
	}

	return defaultCfg, nil
}
