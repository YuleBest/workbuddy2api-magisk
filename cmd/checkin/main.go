package main

import (
	"os"
	"path/filepath"

	"workbuddy2api/internal/wbapicli"
)

func main() {
	project, err := os.Getwd()
	if err == nil {
		if executable, exeErr := os.Executable(); exeErr == nil {
			if real, realErr := filepath.EvalSymlinks(executable); realErr == nil {
				executable = real
			}
			project = filepath.Dir(executable)
		}
	}
	code := wbapicli.RunCheckin(wbapicli.CheckinOptions{
		Args:    os.Args[1:],
		Env:     wbapicli.CurrentEnv(),
		Project: project,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	os.Exit(code)
}
