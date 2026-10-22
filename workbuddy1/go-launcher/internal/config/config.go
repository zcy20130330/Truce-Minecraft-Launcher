// Package config 负责启动器配置的读写（对应 prototype 的 launcher/config.py）。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 是启动器的全部配置项，JSON 字段名与 Python 原型保持一致，
// 便于两边共用同一份 config.json。
type Config struct {
	McFolder        string  `json:"mc_folder"`
	JavaPath        string  `json:"java_path"`
	RamGB           float64 `json:"ram_gb"`
	AutoMemory      bool    `json:"auto_memory"`
	MemoryOptimize  bool    `json:"memory_optimize"`
	MemoryReserveMB int     `json:"memory_reserve_mb"`
	Language        string  `json:"language"`
	Username        string  `json:"username"`
	WindowWidth     int     `json:"window_width"`
	WindowHeight    int     `json:"window_height"`
	Fullscreen      bool    `json:"fullscreen"`
	VersionIsolate  bool    `json:"version_isolate"`
	JvmArgs         string  `json:"jvm_args"`
	GameArgs        string  `json:"game_args"`
	Server          string  `json:"server"`
	DownloadSource  int     `json:"download_source"`
	CheckFiles      bool    `json:"check_files"`
}

// DefaultConfig 返回默认配置。mc_folder 默认取 %APPDATA%\.minecraft。
func DefaultConfig() *Config {
	return &Config{
		McFolder:        defaultMCFolder(),
		JavaPath:        "",
		RamGB:           4.0,
		AutoMemory:      true,
		MemoryOptimize:  true,
		MemoryReserveMB: 1024,
		Language:        "system",
		Username:        "Player",
		WindowWidth:     854,
		WindowHeight:    480,
		Fullscreen:      false,
		VersionIsolate:  false,
		JvmArgs:         "",
		GameArgs:        "",
		Server:          "",
		DownloadSource:  0,
		CheckFiles:      true,
	}
}

func defaultMCFolder() string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, ".minecraft")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".minecraft"
	}
	return filepath.Join(home, ".minecraft")
}

// Path 返回配置文件路径：<可执行目录>/config.json。
// 与 Python 原型一致——配置放在工程目录而非用户目录，便于多份配置并存。
func Path(exeDir string) string {
	return filepath.Join(exeDir, "config.json")
}

// Load 读取配置；文件不存在或损坏时返回默认配置（不报错，与原型一致）。
func Load(path string) *Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	// 先用默认值填充，再用文件内容覆盖，保证新增字段有默认值
	if err := json.Unmarshal(data, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[config] 读取配置失败，使用默认值：%v\n", err)
		return DefaultConfig()
	}
	if cfg.McFolder == "" {
		cfg.McFolder = defaultMCFolder()
	}
	return cfg
}

// Save 写回配置。
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
