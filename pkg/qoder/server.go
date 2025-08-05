package qoder

import (
	"context"

	"github.com/google/go-github/v73/github"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/oauth2"
)

// NewServer creates a new Qoder MCP server with the specified configuration
func NewServer(version, token, owner, repo, commentID, commentType string) *server.MCPServer {
	// Create a new MCP server
	s := server.NewMCPServer(
		"qoder-github-mcp-server",
		version,
	)

	// Create GitHub client factory
	getClient := func(ctx context.Context) (*github.Client, error) {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		return github.NewClient(tc), nil
	}

	// Register tools
	registerTools(s, getClient, owner, repo, commentID, commentType)

	return s
}

// registerTools registers all available tools with the MCP server
func registerTools(s *server.MCPServer, getClient GetClientFn, owner, repo, commentID, commentType string) {
	// Register the comment update tool (supports both issue and review comments)
	tool, handler := QoderUpdateComment(getClient, owner, repo, commentID, commentType)
	s.AddTool(tool, handler)

	// Future tools can be added here:
	// tool2, handler2 := AnotherQoderTool(getClient, ...)
	// s.AddTool(tool2, handler2)
}

// GetClientFn is a function type for getting a GitHub client
type GetClientFn func(context.Context) (*github.Client, error)
