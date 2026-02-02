package main

import (
	"os"

	"github.com/autobrr/go-mediainfo/internal/cli"
)

func main() {
	exitCode := cli.Run(os.Args, os.Stdout, os.Stderr)
	os.Exit(exitCode)
}
