package wbapicli

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// Env preserves whether a variable was explicitly set, including an empty value.
type Env map[string]string

// CurrentEnv snapshots the process environment, dropping duplicate keys the same
// way os.Getenv resolves them.
func CurrentEnv() Env {
	out := make(Env)
	for _, item := range os.Environ() {
		for i := 0; i < len(item); i++ {
			if item[i] == '=' {
				out[item[:i]] = item[i+1:]
				break
			}
		}
	}
	return out
}

func (e Env) get(key string) (string, bool) {
	v, ok := e[key]
	return v, ok
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Runner interface {
	Run(name string, args ...string) CommandResult
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) CommandResult {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 0}
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 127}
	}
	return CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitErr.ExitCode()}
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm fs.FileMode) error
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error)
	Chmod(name string, mode fs.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (OSFileSystem) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (OSFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (OSFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (OSFileSystem) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}

func (OSFileSystem) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(name, mode)
}

func (OSFileSystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (OSFileSystem) Remove(name string) error {
	return os.Remove(name)
}

type ProcessHandle interface {
	PID() int
	Signal(sig os.Signal) error
	Release() error
}

type Platform interface {
	GOOS() string
	ProcessAlive(pid int) bool
	Terminate(pid int) error
	StartProcess(name string, args []string, dir string, stdout, stderr io.Writer) (ProcessHandle, error)
	UserID(name string) (int, error)
	FileOwnerUID(name string) (int, error)
	Readable(name string) bool
	Sleep(d time.Duration)
}

type Dependencies struct {
	HTTPClient HTTPClient
	FS         FileSystem
	Runner     Runner
	Platform   Platform
	Now        func() time.Time
	Random     io.Reader
}

func (d Dependencies) withDefaults() Dependencies {
	if d.HTTPClient == nil {
		d.HTTPClient = http.DefaultClient
	}
	if d.FS == nil {
		d.FS = OSFileSystem{}
	}
	if d.Runner == nil {
		d.Runner = ExecRunner{}
	}
	if d.Platform == nil {
		d.Platform = defaultPlatform{}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Random == nil {
		d.Random = cryptoRandomReader{}
	}
	return d
}

type Options struct {
	Args    []string
	Env     Env
	Project string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Deps    Dependencies
}

type App struct {
	env         Env
	project     string
	configPath  string
	publicHost  string
	cfg         map[string]any
	cfgRaw      []byte
	cfgLoaded   bool
	cfgWarn     error
	quietConfig bool
	deps        Dependencies
	stdout      io.Writer
	stderr      io.Writer
	stdin       io.Reader
	osName      string
	now         time.Time
}
