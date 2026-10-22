package main

import (
	"os"
	"path/filepath"

	"workbuddy2api/internal/wbapicli"
)

func main() {
	project := "."
	if executable, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(executable); err == nil {
			executable = real
		}
		project = filepath.Dir(executable)
	}
	code := wbapicli.Run(wbapicli.Options{
		Args:    os.Args[1:],
		Env:     wbapicli.CurrentEnv(),
		Project: project,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	os.Exit(code)
}
