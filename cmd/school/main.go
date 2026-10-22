package main

import (
	"os"

	"workbuddy2api/internal/tasks"
)

func main() {
	os.Exit(tasks.SchoolCLI(os.Args[1:], os.Stdout, os.Stderr, tasks.NewService()))
}
