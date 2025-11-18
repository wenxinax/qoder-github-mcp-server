package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

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

			// Parse GITHUB_REPOSITORY (format: owner/repo)
			repository := viper.GetString("github_repository")
			if repository == "" {
				return errors.New("GITHUB_REPOSITORY not set")
			}
			parts := strings.Split(repository, "/")
			if len(parts) != 2 {
				return fmt.Errorf("GITHUB_REPOSITORY must be in format 'owner/repo', got: %s", repository)
			}

			// Get optional GitHub Actions context
			runID := viper.GetString("github_run_id")
			serverURL := viper.GetString("github_server_url")

			stdioServerConfig := qmcp.StdioServerConfig{
				Version:   version,
				Token:     token,
				Owner:     parts[0],
				Repo:      parts[1],
				RunID:     runID,
				ServerURL: serverURL,
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
	viper.BindEnv("github_repository", "GITHUB_REPOSITORY")
	viper.BindEnv("github_run_id", "GITHUB_RUN_ID")
	viper.BindEnv("github_server_url", "GITHUB_SERVER_URL")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
