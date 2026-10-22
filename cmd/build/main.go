// Command build builds all runtime binaries and the compatibility command names.
//
// It intentionally uses only the Go toolchain. The generated *.sh files are Go
// executables, not shell scripts, so existing Unix command names keep working
// without keeping shell sources in the repository.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type buildSpec struct {
	Name string
	Pkg  string
}

type aliasSpec struct {
	Name   string
	Target string
}

var buildSpecs = []buildSpec{
	{Name: "wb2api", Pkg: "./cmd/server"},
	{Name: "wbapi", Pkg: "./cmd/wbapi"},
	{Name: "login", Pkg: "./cmd/login"},
	{Name: "signin_bin", Pkg: "./cmd/signin"},
	{Name: "credit", Pkg: "./cmd/credit"},
	{Name: "trial_bin", Pkg: "./cmd/trial"},
	{Name: "acct", Pkg: "./cmd/acct"},
	{Name: "activity", Pkg: "./cmd/activity"},
	{Name: "stats", Pkg: "./cmd/stats"},
	{Name: "checkin", Pkg: "./cmd/checkin"},
	{Name: "dev", Pkg: "./cmd/dev"},
	{Name: "school_open_day_2026", Pkg: "./cmd/school"},
	{Name: "task_runner", Pkg: "./cmd/taskrunner"},
	{Name: "global_region", Pkg: "./cmd/region"},
	{Name: "school_open_day_cron", Pkg: "./cmd/taskcron"},
}

var unixAliases = []aliasSpec{
	{Name: "server", Target: "wb2api"},
	{Name: "login.sh", Target: "login"},
	{Name: "signin", Target: "signin_bin"},
	{Name: "signin.sh", Target: "signin_bin"},
	{Name: "credit.sh", Target: "credit"},
	{Name: "trial", Target: "trial_bin"},
	{Name: "trial.sh", Target: "trial_bin"},
	{Name: "acct.sh", Target: "acct"},
	{Name: "checkin.sh", Target: "checkin"},
	{Name: "dev.sh", Target: "dev"},
	{Name: "school_open_day_cron.sh", Target: "school_open_day_cron"},
}

var windowsAliases = []aliasSpec{
	{Name: "server", Target: "wb2api"},
	{Name: "signin", Target: "signin_bin"},
	{Name: "trial", Target: "trial_bin"},
	{Name: "activity_bin", Target: "activity"},
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out", "dist", "output directory")
	rootFlag := fs.String("root", "", "module root (default: discover from cwd)")
	goBin := fs.String("go", "go", "go command")
	ldflags := fs.String("ldflags", "-s -w", "Go linker flags")
	list := fs.Bool("list", false, "list build targets and aliases")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	goos := firstNonEmpty(os.Getenv("TARGETOS"), os.Getenv("GOOS"), runtime.GOOS)
	goarch := firstNonEmpty(os.Getenv("TARGETARCH"), os.Getenv("GOARCH"), runtime.GOARCH)
	if *list {
		printTargets(stdout, goos)
		return nil
	}

	root := *rootFlag
	if root == "" {
		var err error
		root, err = findModuleRoot()
		if err != nil {
			return err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(*outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	for _, spec := range buildSpecs {
		target := executableName(spec.Name, goos)
		path := filepath.Join(out, target)
		fmt.Fprintf(stdout, "build %-28s <- %s\n", target, spec.Pkg)
		if err := runGoBuild(*goBin, root, path, spec.Pkg, goos, goarch, *ldflags, stderr); err != nil {
			return fmt.Errorf("build %s: %w", spec.Pkg, err)
		}
	}

	aliases := unixAliases
	if goos == "windows" {
		aliases = windowsAliases
	}
	for _, alias := range aliases {
		src := filepath.Join(out, executableName(alias.Target, goos))
		dst := filepath.Join(out, executableName(alias.Name, goos))
		if err := copyExecutable(src, dst); err != nil {
			return fmt.Errorf("create compatibility command %s: %w", alias.Name, err)
		}
	}
	return nil
}

func runGoBuild(goBin, root, output, pkg, goos, goarch, ldflags string, stderr io.Writer) error {
	cmd := exec.Command(goBin, "build", "-trimpath", "-ldflags", ldflags, "-o", output, pkg)
	cmd.Dir = root
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"GOOS":        goos,
		"GOARCH":      goarch,
		"CGO_ENABLED": "0",
	})
	return cmd.Run()
}

func withEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if _, replace := values[key]; !replace {
			out = append(out, item)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func executableName(name, goos string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func printTargets(w io.Writer, goos string) {
	for _, spec := range buildSpecs {
		fmt.Fprintf(w, "%s\t%s\n", executableName(spec.Name, goos), spec.Pkg)
	}
	aliases := unixAliases
	if goos == "windows" {
		aliases = windowsAliases
	}
	for _, alias := range aliases {
		fmt.Fprintf(w, "%s\tcompat:%s\n", executableName(alias.Name, goos), alias.Target)
	}
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
