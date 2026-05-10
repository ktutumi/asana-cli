package main

import (
	"os"

	"github.com/ktutumi/asana-cli-go/internal/cli"
)

func main() {
	os.Exit(cli.RunCLI(os.Args[1:], cli.NewStdIO(), cli.NewRuntimeOptionsFromEnv()))
}
