package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/autobrr/go-mediainfo/internal/cli"
)

var version = "dev"

const helpBanner = "" +
	"                                                                                \n" +
	"███    ███ ███████ ██████  ██  █████  ██ ███    ██ ███████  ██████\n" +
	"████  ████ ██      ██   ██ ██ ██   ██ ██ ████   ██ ██      ██    ██\n" +
	"██ ████ ██ █████   ██   ██ ██ ███████ ██ ██ ██  ██ █████   ██    ██\n" +
	"██  ██  ██ ██      ██   ██ ██ ██   ██ ██ ██  ██ ██ ██      ██    ██\n" +
	"██      ██ ███████ ██████  ██ ██   ██ ██ ██   ████ ██       ██████"

const helpTemplate = helpBanner + `

{{with or .Long .Short}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

var rootCmd = &cobra.Command{
	Use:                "mediainfo [options] <file> [file...]",
	Short:              "Go rewrite of MediaInfo CLI.",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			_ = cmd.Help()
			return
		}
		exitCode := cli.Run(append([]string{cmd.Name()}, args...), cmd.OutOrStdout(), cmd.ErrOrStderr())
		os.Exit(exitCode)
	},
}

func init() {
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	rootCmd.SetHelpTemplate(helpTemplate)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
