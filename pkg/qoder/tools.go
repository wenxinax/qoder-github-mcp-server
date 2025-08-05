package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-github/v73/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// QoderUpdateComment creates a tool to update a comment (issue or review) with content between Qoder markers
func QoderUpdateComment(getClient GetClientFn, owner, repo, commentID, commentType string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "qoder_update_comment"
	description := fmt.Sprintf("Update a %s comment by replacing content between <!-- QODER_BODY_START --> and <!-- QODER_BODY_END --> markers.", commentType)

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("new_content",
				mcp.Required(),
				mcp.Description("New content to replace between the Qoder markers"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters
			newContent, err := getRequiredStringParam(request, "new_content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse comment ID from environment variable
			commentIDInt, err := strconv.ParseInt(commentID, 10, 64)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid comment ID: %v", err)), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Handle different comment types
			switch commentType {
			case "issue":
				return updateIssueComment(ctx, client, owner, repo, commentIDInt, newContent)
			case "review":
				return updateReviewComment(ctx, client, owner, repo, commentIDInt, newContent)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("unsupported comment type: %s", commentType)), nil
			}
		}
}

// updateIssueComment updates an issue comment
func updateIssueComment(ctx context.Context, client *github.Client, owner, repo string, commentID int64, newContent string) (*mcp.CallToolResult, error) {
	// Get the current comment
	comment, _, err := client.Issues.GetComment(ctx, owner, repo, commentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get issue comment: %v", err)), nil
	}

	if comment.Body == nil {
		return mcp.NewToolResultError("comment body is nil"), nil
	}

	// Replace content between markers
	updatedBody, err := replaceQoderContent(*comment.Body, newContent)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to replace content: %v", err)), nil
	}

	// Update the comment
	updateComment := &github.IssueComment{
		Body: github.Ptr(updatedBody),
	}

	updatedComment, _, err := client.Issues.EditComment(ctx, owner, repo, commentID, updateComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update issue comment: %v", err)), nil
	}

	// Return the updated comment as JSON
	result, err := json.Marshal(updatedComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// updateReviewComment updates a pull request review comment
func updateReviewComment(ctx context.Context, client *github.Client, owner, repo string, commentID int64, newContent string) (*mcp.CallToolResult, error) {
	// Get the current review comment
	comment, _, err := client.PullRequests.GetComment(ctx, owner, repo, commentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get review comment: %v", err)), nil
	}

	if comment.Body == nil {
		return mcp.NewToolResultError("comment body is nil"), nil
	}

	// Replace content between markers
	updatedBody, err := replaceQoderContent(*comment.Body, newContent)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to replace content: %v", err)), nil
	}

	// Update the comment
	updateComment := &github.PullRequestComment{
		Body: github.Ptr(updatedBody),
	}

	updatedComment, _, err := client.PullRequests.EditComment(ctx, owner, repo, commentID, updateComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to update review comment: %v", err)), nil
	}

	// Return the updated comment as JSON
	result, err := json.Marshal(updatedComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// replaceQoderContent replaces content between Qoder markers
func replaceQoderContent(originalBody, newContent string) (string, error) {
	const startMarker = "<!-- QODER_BODY_START -->"
	const endMarker = "<!-- QODER_BODY_END -->"

	startIndex := strings.Index(originalBody, startMarker)
	if startIndex == -1 {
		return "", fmt.Errorf("start marker '%s' not found", startMarker)
	}

	endIndex := strings.Index(originalBody, endMarker)
	if endIndex == -1 {
		return "", fmt.Errorf("end marker '%s' not found", endMarker)
	}

	if endIndex <= startIndex {
		return "", fmt.Errorf("end marker appears before start marker")
	}

	// Build the new body
	before := originalBody[:startIndex+len(startMarker)]
	after := originalBody[endIndex:]
	
	return before + "\n" + newContent + "\n" + after, nil
}

// getRequiredStringParam extracts a required string parameter from the request
func getRequiredStringParam(request mcp.CallToolRequest, paramName string) (string, error) {
	value := request.GetString(paramName, "")
	if value == "" {
		return "", fmt.Errorf("required parameter '%s' not provided or empty", paramName)
	}
	return value, nil
}
