package qmcp

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/qoder/qoder-github-mcp-server/pkg/qoder"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
)

type StdioServerConfig struct {
	// Version of the server
	Version string

	// GitHub Token to authenticate with the GitHub API
	Token string

	// GitHub repository owner
	Owner string

	// GitHub repository name
	Repo string
}

// RunStdioServer starts the MCP server with stdio transport
func RunStdioServer(cfg StdioServerConfig) error {
	// Create app context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Create the MCP server
	qoderServer := qoder.NewServer(cfg.Version, cfg.Token, cfg.Owner, cfg.Repo)

	// Create stdio server
	stdioServer := server.NewStdioServer(qoderServer)

	// Setup logging
	logrusLogger := logrus.New()
	logrusLogger.SetLevel(logrus.InfoLevel)
	stdLogger := log.New(logrusLogger.Writer(), "qoder-mcp-server", 0)
	stdioServer.SetErrorLogger(stdLogger)

	// Start listening for messages
	errC := make(chan error, 1)
	go func() {
		in, out := io.Reader(os.Stdin), io.Writer(os.Stdout)
		errC <- stdioServer.Listen(ctx, in, out)
	}()

	// Output server start message to stderr (so it doesn't interfere with stdio communication)
	_, _ = fmt.Fprintf(os.Stderr, "Qoder GitHub MCP Server running on stdio\n")

	// Wait for shutdown signal
	select {
	case <-ctx.Done():
		logrusLogger.Infof("shutting down server...")
	case err := <-errC:
		if err != nil {
			return fmt.Errorf("error running server: %w", err)
		}
	}

	return nil
}
