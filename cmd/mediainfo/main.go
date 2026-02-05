package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"

	"github.com/autobrr/go-mediainfo/internal/cli"
)

var version = "dev"

const helpBanner = "" +
	"                                                                                \n" +
	"███╗   ███╗███████╗██████╗ ██╗ █████╗ ██╗███╗   ██╗███████╗ ██████╗ \n" +
	"████╗ ████║██╔════╝██╔══██╗██║██╔══██╗██║████╗  ██║██╔════╝██╔═══██╗\n" +
	"██╔████╔██║█████╗  ██║  ██║██║███████║██║██╔██╗ ██║█████╗  ██║   ██║\n" +
	"██║╚██╔╝██║██╔══╝  ██║  ██║██║██╔══██║██║██║╚██╗██║██╔══╝  ██║   ██║\n" +
	"██║ ╚═╝ ██║███████╗██████╔╝██║██║  ██║██║██║ ╚████║██║     ╚██████╔╝\n" +
	"╚═╝     ╚═╝╚══════╝╚═════╝ ╚═╝╚═╝  ╚═╝╚═╝╚═╝  ╚═══╝╚═╝      ╚═════╝ "

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
		if hasUpdateFlag(args) {
			if err := runSelfUpdate(cmd.Context()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "mediainfo: %s\n", err.Error())
				os.Exit(1)
			}
			return
		}
		if len(args) == 0 {
			_ = cmd.Help()
			return
		}
		exitCode := cli.Run(append([]string{cmd.Name()}, args...), cmd.OutOrStdout(), cmd.ErrOrStderr())
		os.Exit(exitCode)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update mediainfo",
	Long:  "Update mediainfo to latest version (release builds only).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSelfUpdate(cmd.Context())
	},
	DisableFlagsInUseLine: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "mediainfo version: %s\n", version)
		return nil
	},
	DisableFlagsInUseLine: true,
}

func init() {
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func hasUpdateFlag(args []string) bool {
	for _, arg := range args {
		n := normalizeArgName(arg)
		if n == "--self-update" || n == "--update" {
			return true
		}
	}
	return false
}

func normalizeArgName(arg string) string {
	eq := strings.IndexByte(arg, '=')
	if eq == -1 {
		eq = len(arg)
	}
	return strings.ToLower(arg[:eq])
}

func runSelfUpdate(ctx context.Context) error {
	if version == "" || version == "dev" {
		return errors.New("self-update is only available in release builds")
	}

	if _, err := semver.ParseTolerant(version); err != nil {
		return fmt.Errorf("could not parse version: %w", err)
	}

	latest, found, err := selfupdate.DetectLatest(ctx, selfupdate.ParseSlug("autobrr/go-mediainfo"))
	if err != nil {
		return fmt.Errorf("error occurred while detecting version: %w", err)
	}
	if !found {
		return fmt.Errorf("latest version for %s/%s could not be found from github repository", "autobrr/go-mediainfo", version)
	}

	if latest.LessOrEqual(version) {
		fmt.Printf("Current binary is the latest version: %s\n", version)
		return nil
	}

	exe, err := selfupdate.ExecutablePath()
	if err != nil {
		return fmt.Errorf("could not locate executable path: %w", err)
	}

	if err := selfupdate.UpdateTo(ctx, latest.AssetURL, latest.AssetName, exe); err != nil {
		return fmt.Errorf("error occurred while updating binary: %w", err)
	}

	fmt.Printf("Successfully updated to version: %s\n", latest.Version())
	return nil
}
