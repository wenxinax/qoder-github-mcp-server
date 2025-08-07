package qoder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/go-github/v73/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// QoderFixContext holds the context for a one-click Qoder fix
type QoderFixContext struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	PullNumber int    `json:"pull_number"`
	CommitID   string `json:"commit_id"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Side       string `json:"side"`
	StartLine  int    `json:"start_line,omitempty"`
	StartSide  string `json:"start_side,omitempty"`
	Body       string `json:"body"`
}

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

// QoderAddCommentToPendingReview creates a tool to add a review comment to a pending review
func QoderAddCommentToPendingReview(getClient GetClientFn) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "qoder_add_comment_to_pending_review"
	description := "Add review comment to the requester's latest pending pull request review. It automatically finds the latest commit."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("body", mcp.Required(), mcp.Description("The text of the review comment")),
			mcp.WithNumber("line", mcp.Required(), mcp.Description("The line of the blob in the pull request diff that the comment applies to")),
			mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
			mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file that necessitates a comment")),
			mcp.WithNumber("pullNumber", mcp.Required(), mcp.Description("Pull request number")),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("side", mcp.Required(), mcp.Description("The side of the diff to comment on. Can be LEFT or RIGHT")),
			mcp.WithNumber("startLine", mcp.Description("For multi-line comments, the first line of the range that the comment applies to")),
			mcp.WithString("startSide", mcp.Description("For multi-line comments, the starting side of the diff that the comment applies to. Can be LEFT or RIGHT")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			body, err := getRequiredStringParam(request, "body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			line, err := getRequiredIntParam(request, "line")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			owner, err := getRequiredStringParam(request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, err := getRequiredStringParam(request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pullNumber, err := getRequiredIntParam(request, "pullNumber")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := getRequiredStringParam(request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			side, err := getRequiredStringParam(request, "side")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Automatically get the head commit SHA of the pull request
			pr, _, err := client.PullRequests.Get(ctx, owner, repo, pullNumber)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get pull request details: %v", err)), nil
			}
			commitID := pr.GetHead().GetSHA()
			if commitID == "" {
				return mcp.NewToolResultError("could not get head commit SHA from pull request"), nil
			}

			// Create QoderFixContext for the footer link
			fixContext := QoderFixContext{
				Owner:      owner,
				Repo:       repo,
				PullNumber: pullNumber,
				CommitID:   commitID,
				Path:       path,
				Line:       line,
				Side:       side,
				Body:       body,
			}

			// Add optional multi-line parameters to the context
			if startLine := request.GetFloat("startLine", 0); startLine > 0 {
				fixContext.StartLine = int(startLine)
				if startSide := request.GetString("startSide", ""); startSide != "" {
					fixContext.StartSide = startSide
				} else {
					fixContext.StartSide = side // Default to the same side
				}
			}

			// Marshal and encode the context for the footer
			contextJSON, err := json.Marshal(fixContext)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal context: %v", err)), nil
			}
			encodedContext := base64.StdEncoding.EncodeToString(contextJSON)
			footer := fmt.Sprintf(`

---
*Powered by Qoder* | [One-Click Qoder Fix](http://localhost:9080/reload-to-qoder?context=%s)`, encodedContext)
			fullBody := body + footer

			// Find pending review by app login name
			const appLogin = "qoder-assist[bot]"
			reviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, pullNumber, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list reviews: %v", err)), nil
			}

			var pendingReviewID int64
			for _, review := range reviews {
				if review.GetState() == "PENDING" && review.User != nil && review.User.GetLogin() == appLogin {
					pendingReviewID = review.GetID()
					break
				}
			}

			if pendingReviewID != 0 {
				// CASE 1: A pending review already exists. Add a comment to it.
				comment := &github.PullRequestComment{
					Body:                github.String(fullBody),
					CommitID:            github.String(commitID),
					Path:                github.String(path),
					Line:                github.Int(line),
					Side:                github.String(side),
					PullRequestReviewID: github.Int64(pendingReviewID),
				}
				if fixContext.StartLine > 0 {
					comment.StartLine = github.Int(fixContext.StartLine)
					comment.StartSide = github.String(fixContext.StartSide)
				}

				createdComment, _, err := client.PullRequests.CreateComment(ctx, owner, repo, pullNumber, comment)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to create review comment: %v", err)), nil
				}
				result, err := json.Marshal(createdComment)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
				}
				return mcp.NewToolResultText(string(result)), nil
			} else {
				// CASE 2: No pending review exists. Create a new one with this comment.
				draftComment := &github.DraftReviewComment{
					Path: github.String(path),
					Body: github.String(fullBody),
					Line: github.Int(line),
					Side: github.String(side),
				}
				if fixContext.StartLine > 0 {
					draftComment.StartLine = github.Int(fixContext.StartLine)
					draftComment.StartSide = github.String(fixContext.StartSide)
				}

				reviewRequest := &github.PullRequestReviewRequest{
					Event:    github.String("PENDING"),
					Comments: []*github.DraftReviewComment{draftComment},
					CommitID: github.String(commitID),
				}
				newReview, _, err := client.PullRequests.CreateReview(ctx, owner, repo, pullNumber, reviewRequest)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to create new pending review: %v", err)), nil
				}
				result, err := json.Marshal(newReview)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
				}
				return mcp.NewToolResultText(string(result)), nil
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

// getRequiredIntParam extracts a required integer parameter from the request
func getRequiredIntParam(request mcp.CallToolRequest, paramName string) (int, error) {
	value := request.GetFloat(paramName, 0)
	if value == 0 {
		// This is not a robust check, but it's the best we can do with the mcp-go library
		return 0, fmt.Errorf("required integer parameter '%s' not provided or is zero", paramName)
	}
	return int(value), nil
}

// getOptionalIntParam extracts an optional integer parameter from the request
func getOptionalIntParam(request mcp.CallToolRequest, paramName string) int {
	return int(request.GetFloat(paramName, 0))
}

// getRequiredNumberParam extracts a required number parameter from the request
func getRequiredNumberParam(request mcp.CallToolRequest, paramName string) (int, error) {
	value := request.GetFloat(paramName, 0)
	if value == 0 {
		// This is not a robust check, but it's the best we can do with the mcp-go library
		return 0, fmt.Errorf("required number parameter '%s' not provided or is zero", paramName)
	}
	return int(value), nil
}

// getOptionalNumberParam extracts an optional number parameter from the request
func getOptionalNumberParam(request mcp.CallToolRequest, paramName string) int {
	return int(request.GetFloat(paramName, 0))
}
