package qoder

import (
	"context"

	"github.com/google/go-github/v73/github"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// NewServer creates a new Qoder MCP server with the specified configuration
func NewServer(version, token, owner, repo string) *server.MCPServer {
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

	getGQLClient := func(ctx context.Context) (*githubv4.Client, error) {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc := oauth2.NewClient(ctx, ts)
		return githubv4.NewClient(tc), nil
	}

	// Register tools
	registerTools(s, getClient, getGQLClient, owner, repo)

	return s
}

// registerTools registers all available tools with the MCP server
func registerTools(s *server.MCPServer, getClient GetClientFn, getGQLClient GetGQLClientFn, owner, repo string) {
	// Register the add review line comment tool
	addCommentTool, addCommentHandler := AddCommentToPendingReview(getClient, getGQLClient, owner, repo)
	s.AddTool(addCommentTool, addCommentHandler)

	// Register the create pending review tool
	createReviewTool, createReviewHandler := CreatePendingPullRequestReview(getClient, owner, repo)
	s.AddTool(createReviewTool, createReviewHandler)

	// Register the submit pending review tool
	submitReviewTool, submitReviewHandler := SubmitPendingPullRequestReview(getClient, getGQLClient, owner, repo)
	s.AddTool(submitReviewTool, submitReviewHandler)

	// Register the reply comment tool
	replyCommentTool, replyCommentHandler := ReplyComment(getClient, owner, repo)
	s.AddTool(replyCommentTool, replyCommentHandler)

	// Register the update comment tool
	updateCommentTool, updateCommentHandler := UpdateComment(getClient, owner, repo)
	s.AddTool(updateCommentTool, updateCommentHandler)

	// Register the get PR diff tool (with line numbers and compression)
	getPRDiffTool, getPRDiffHandler := GetPullRequestDiff(getClient, owner, repo)
	s.AddTool(getPRDiffTool, getPRDiffHandler)

	// Register the get PR files tool
	getPRFilesTool, getPRFilesHandler := GetPullRequestFiles(getClient, owner, repo)
	s.AddTool(getPRFilesTool, getPRFilesHandler)

	// Register the get pull request tool
	getPullRequestTool, getPullRequestHandler := GetPullRequest(getClient, owner, repo)
	s.AddTool(getPullRequestTool, getPullRequestHandler)

	// Register the get pull request comments tool
	getPullRequestCommentsTool, getPullRequestCommentsHandler := GetPullRequestComments(getClient, owner, repo)
	s.AddTool(getPullRequestCommentsTool, getPullRequestCommentsHandler)

	// Register the get pull request reviews tool
	getPullRequestReviewsTool, getPullRequestReviewsHandler := GetPullRequestReviews(getClient, owner, repo)
	s.AddTool(getPullRequestReviewsTool, getPullRequestReviewsHandler)

	// Register the create or update file tool
	createOrUpdateFileTool, createOrUpdateFileHandler := CreateOrUpdateFile(getClient, owner, repo)
	s.AddTool(createOrUpdateFileTool, createOrUpdateFileHandler)

	// Register the push files tool
	pushFilesTool, pushFilesHandler := PushFiles(getClient, owner, repo)
	s.AddTool(pushFilesTool, pushFilesHandler)

	// Register the create branch tool
	createBranchTool, createBranchHandler := CreateBranch(getClient, owner, repo)
	s.AddTool(createBranchTool, createBranchHandler)

	// Register the create pull request tool
	createPullRequestTool, createPullRequestHandler := CreatePullRequest(getClient, owner, repo)
	s.AddTool(createPullRequestTool, createPullRequestHandler)
}
