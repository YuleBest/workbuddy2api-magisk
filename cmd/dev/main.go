package main

import (
	"os"
	"path/filepath"

	"workbuddy2api/internal/wbapicli"
)

func main() {
	project, _ := os.Getwd()
	if executable, err := os.Executable(); err == nil {
		if real, realErr := filepath.EvalSymlinks(executable); realErr == nil {
			executable = real
		}
		project = filepath.Dir(executable)
	}
	code := wbapicli.RunDev(wbapicli.DevOptions{
		Args:    os.Args[1:],
		Env:     wbapicli.CurrentEnv(),
		Project: project,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	os.Exit(code)
}
