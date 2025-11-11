package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/qoder/qoder-github-mcp-server/internal/qmcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// These variables are set by the build process using ldflags.
var version = "dev"
var commit = "unknown"
var date = "unknown"

var (
	rootCmd = &cobra.Command{
		Use:     "qoder-github-mcp-server",
		Short:   "Qoder GitHub MCP Server",
		Long:    `A specialized GitHub MCP server for Qoder operations.`,
		Version: fmt.Sprintf("Version: %s\nCommit: %s\nBuild Date: %s", version, commit, date),
	}

	stdioCmd = &cobra.Command{
		Use:   "stdio",
		Short: "Start stdio server",
		Long:  `Start a server that communicates via standard input/output streams using JSON-RPC messages.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			token := viper.GetString("github_token")
			if token == "" {
				return errors.New("GITHUB_TOKEN not set")
			}

			owner := viper.GetString("github_owner")
			if owner == "" {
				return errors.New("GITHUB_OWNER not set")
			}

			repo := viper.GetString("github_repo")
			if repo == "" {
				return errors.New("GITHUB_REPO not set")
			}

			stdioServerConfig := qmcp.StdioServerConfig{
				Version: version,
				Token:   token,
				Owner:   owner,
				Repo:    repo,
			}
			return qmcp.RunStdioServer(stdioServerConfig)
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.SetVersionTemplate("{{.Short}}\n{{.Version}}\n")
	rootCmd.AddCommand(stdioCmd)
}

func initConfig() {
	viper.SetEnvPrefix("") // No prefix for environment variables
	viper.AutomaticEnv()

	// Bind environment variables
	viper.BindEnv("github_token", "GITHUB_TOKEN")
	viper.BindEnv("github_owner", "GITHUB_OWNER")
	viper.BindEnv("github_repo", "GITHUB_REPO")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
