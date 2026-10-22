package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const moduleID = "wb2api"

type moduleConfig struct {
	Listen    string `json:"listen"`
	APIKey    string `json:"api_key"`
	KeysFile  string `json:"keys_file"`
	AuthDir   string `json:"auth_dir"`
	StateFile string `json:"state_file"`
	Admin     struct {
		Enabled bool   `json:"enabled"`
		Token   string `json:"token"`
	} `json:"admin"`
}

type moduleEnv struct {
	ModuleDir string
	Config    moduleConfig

	ConfigPath string
	ServerBin  string
	LoginBin   string
	AuthDir    string
	KeysFile   string
	StateFile  string
	LogDir     string
	LogFile    string
	RunDir     string
	PIDFile    string
	TmpDir     string
}

func loadEnv() (*moduleEnv, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("定位 wbapi: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, fmt.Errorf("解析 wbapi 路径: %w", err)
	}
	binDir := filepath.Dir(exe)
	moduleDir := filepath.Dir(binDir)
	if filepath.Base(binDir) != "bin" {
		return nil, fmt.Errorf("wbapi 必须位于模块 bin 目录: %s", exe)
	}

	configPath := filepath.Join(moduleDir, "config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", configPath, err)
	}
	var cfg moduleConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", configPath, err)
	}

	authDir, err := managedPath(moduleDir, cfg.AuthDir, "auths")
	if err != nil {
		return nil, fmt.Errorf("auth_dir: %w", err)
	}
	keysFile, err := managedPath(moduleDir, cfg.KeysFile, "keys.json")
	if err != nil {
		return nil, fmt.Errorf("keys_file: %w", err)
	}
	stateFile, err := managedPath(moduleDir, cfg.StateFile, filepath.Join("data", "state.json"))
	if err != nil {
		return nil, fmt.Errorf("state_file: %w", err)
	}

	return &moduleEnv{
		ModuleDir:  moduleDir,
		Config:     cfg,
		ConfigPath: configPath,
		ServerBin:  filepath.Join(moduleDir, "bin", "wb2api-server"),
		LoginBin:   filepath.Join(moduleDir, "bin", "wb2api-login"),
		AuthDir:    authDir,
		KeysFile:   keysFile,
		StateFile:  stateFile,
		LogDir:     filepath.Join(moduleDir, "logs"),
		LogFile:    filepath.Join(moduleDir, "logs", "gateway.log"),
		RunDir:     filepath.Join(moduleDir, "run"),
		PIDFile:    filepath.Join(moduleDir, "run", "wb2api.pid"),
		TmpDir:     filepath.Join(moduleDir, "tmp"),
	}, nil
}

func managedPath(moduleDir, configured, fallback string) (string, error) {
	value := strings.TrimSpace(configured)
	if value == "" {
		value = fallback
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(moduleDir, value)
	}
	cleanDir, err := filepath.Abs(moduleDir)
	if err != nil {
		return "", err
	}
	cleanPath, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径必须在模块目录内: %s", cleanPath)
	}
	return cleanPath, nil
}

func (e *moduleEnv) baseURL() string {
	listen := strings.TrimSpace(e.Config.Listen)
	if listen == "" {
		listen = "127.0.0.1:7863"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		if strings.HasPrefix(listen, ":") {
			host, port = "", strings.TrimPrefix(listen, ":")
		} else if _, err := strconv.Atoi(listen); err == nil {
			host, port = "127.0.0.1", listen
		} else {
			return "http://127.0.0.1:7863"
		}
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func (e *moduleEnv) adminURL() string {
	return e.baseURL() + "/admin/"
}

func (e *moduleEnv) listenIsLoopback() bool {
	host, _, err := net.SplitHostPort(e.Config.Listen)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (e *moduleEnv) prepareRuntimeDirs() error {
	for _, dir := range []string{e.AuthDir, filepath.Dir(e.StateFile), e.LogDir, e.RunDir, e.TmpDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
