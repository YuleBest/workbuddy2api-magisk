package main

import (
	"context"
	"os"

	"workbuddy2api/internal/tasks"
)

func main() {
	os.Exit(tasks.TaskCron(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		tasks.NewService(),
		tasks.FindRepoRoot(),
	))
}
