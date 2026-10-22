package wbapicli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type DevOptions struct {
	Args    []string
	Env     Env
	Project string
	Stdout  io.Writer
	Stderr  io.Writer
	Deps    Dependencies
}

const devUsage = "用法: dev {start|stop|restart|status|logs|key}\n"

func RunDev(opts DevOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = discardWriter{}
	}
	if opts.Stderr == nil {
		opts.Stderr = discardWriter{}
	}
	env := cloneEnv(opts.Env)
	app := newApp(Options{
		Env:     env,
		Project: opts.Project,
		Stdout:  opts.Stdout,
		Stderr:  opts.Stderr,
		Deps:    opts.Deps,
	})
	app.project = opts.Project
	app.configPath = joinProject(opts.Project, "config.json")
	app.quietConfig = true
	action := "status"
	if len(opts.Args) > 0 {
		action = opts.Args[0]
	}
	switch action {
	case "start":
		return app.devStart()
	case "stop":
		return app.devStop()
	case "restart":
		code := app.devStop()
		app.deps.Platform.Sleep(time.Second)
		next := app.devStart()
		if next != 0 {
			return next
		}
		return code
	case "status":
		return app.devStatus()
	case "logs":
		return app.devLogs()
	case "key":
		return app.devKey()
	default:
		fmt.Fprint(opts.Stdout, devUsage)
		return 1
	}
}

func (a *App) devPaths() (bin, log, pidfile, config string) {
	bin = joinProject(a.project, "wb2api")
	if a.osName == "windows" {
		bin += ".exe"
	}
	return bin,
		joinProject(a.project, "server.log"),
		joinProject(a.project, ".server.pid"),
		joinProject(a.project, "config.json")
}

func (a *App) devPID(path string) (int, bool) {
	raw, err := a.deps.FS.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, a.deps.Platform.ProcessAlive(pid)
}

func (a *App) devStart() int {
	bin, logPath, pidPath, configPath := a.devPaths()
	displayBin, displayLog := "./wb2api", "./server.log"
	if a.osName == "windows" {
		displayBin = "./wb2api.exe"
	}
	if pid, alive := a.devPID(pidPath); alive {
		fmt.Fprintf(a.stdout, "已在运行 (PID %d)\n", pid)
		return 0
	}
	info, err := a.deps.FS.Stat(bin)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		fmt.Fprintf(a.stdout, "未找到 %s，先执行: CGO_ENABLED=0 go build -o wb2api ./cmd/server\n", displayBin)
		return 1
	}
	logFile, err := a.deps.FS.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		fmt.Fprintf(a.stdout, "启动失败，见 %s\n", displayLog)
		return 1
	}
	proc, err := a.deps.Platform.StartProcess(bin, []string{"-config", configPath}, a.project, logFile, logFile)
	if closeErr := logFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(a.stdout, "启动失败，见 %s\n", displayLog)
		a.tailFile(logPath, 20)
		return 1
	}
	pid := proc.PID()
	_ = proc.Release()
	if err := a.deps.FS.WriteFile(pidPath, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		fmt.Fprintf(a.stdout, "启动失败，见 %s\n", displayLog)
		return 1
	}
	a.deps.Platform.Sleep(2 * time.Second)
	if a.deps.Platform.ProcessAlive(pid) {
		fmt.Fprintf(a.stdout, "已启动 (PID %d)\n", pid)
		return 0
	}
	fmt.Fprintf(a.stdout, "启动失败，见 %s\n", displayLog)
	a.tailFile(logPath, 20)
	return 1
}

func (a *App) devStop() int {
	_, _, pidPath, _ := a.devPaths()
	pid, alive := a.devPID(pidPath)
	if alive {
		if err := a.deps.Platform.Terminate(pid); err != nil {
		} else {
			fmt.Fprintln(a.stdout, "已停止")
		}
	} else {
		fmt.Fprintln(a.stdout, "未在运行")
	}
	_ = a.deps.FS.Remove(pidPath)
	return 0
}

func (a *App) devStatus() int {
	_, _, pidPath, _ := a.devPaths()
	pid, alive := a.devPID(pidPath)
	if !alive {
		fmt.Fprintln(a.stdout, "未运行")
		return 0
	}
	fmt.Fprintf(a.stdout, "运行中 (PID %d)\n", pid)
	body, status, err := a.devHTTP(http.MethodGet, "http://127.0.0.1:7863/healthz", "")
	if err == nil && status != 0 {
		fmt.Fprintln(a.stdout, string(body))
	} else {
		fmt.Fprintln(a.stdout)
	}
	a.loadConfig()
	key := stringValue(a.cfg["api_key"])
	body, status, err = a.devHTTP(http.MethodGet, "http://127.0.0.1:7863/status", key)
	count := "?"
	if err == nil && status == 200 {
		var data map[string]any
		if json.Unmarshal(body, &data) == nil {
			if accounts, ok := data["accounts"].([]any); ok {
				count = strconv.Itoa(len(accounts))
			}
		}
	}
	fmt.Fprintf(a.stdout, "账号数: %s\n", count)
	return 0
}

func (a *App) devLogs() int {
	_, logPath, _, _ := a.devPaths()
	file, err := a.deps.FS.OpenFile(logPath, os.O_RDONLY, 0)
	if err != nil {
		return 1
	}
	defer file.Close()
	raw, _ := io.ReadAll(file)
	fmt.Fprint(a.stdout, tailText(string(raw), 10))
	offset, _ := file.Seek(0, io.SeekEnd)
	for {
		a.deps.Platform.Sleep(500 * time.Millisecond)
		info, err := file.Stat()
		if err != nil {
			return 0
		}
		if info.Size() < offset {
			offset = 0
			_, _ = file.Seek(0, io.SeekStart)
		}
		if info.Size() == offset {
			continue
		}
		_, _ = file.Seek(offset, io.SeekStart)
		chunk, err := io.ReadAll(file)
		if len(chunk) > 0 {
			_, _ = a.stdout.Write(chunk)
			offset += int64(len(chunk))
		}
		if err != nil {
			return 0
		}
	}
}

func (a *App) devKey() int {
	a.loadConfig()
	key, ok := a.cfg["api_key"]
	if !ok {
		fmt.Fprintln(a.stdout, "[!] config.json 没有 api_key")
		return 1
	}
	fmt.Fprintln(a.stdout, pyValue(key))
	return 0
}

func (a *App) tailFile(path string, lines int) {
	raw, err := a.deps.FS.ReadFile(path)
	if err != nil {
		return
	}
	fmt.Fprint(a.stdout, tailText(string(raw), lines))
}

func tailText(text string, lines int) string {
	if lines <= 0 {
		return ""
	}
	items := strings.SplitAfter(text, "\n")
	if len(items) > 0 && items[len(items)-1] == "" {
		items = items[:len(items)-1]
	}
	if len(items) > lines {
		items = items[len(items)-lines:]
	}
	return strings.Join(items, "")
}

func (a *App) devHTTP(method, url, token string) ([]byte, int, error) {
	raw, status, err := a.rawRequest(method, url, token, nil, 5*time.Second)
	return raw, status, err
}
