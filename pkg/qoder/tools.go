package qoder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/go-github/v73/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
)

// GetClientFn is a function type for getting a GitHub client
type GetClientFn func(context.Context) (*github.Client, error)

// GetGQLClientFn is a function type for getting a GitHub GraphQL client
type GetGQLClientFn func(context.Context) (*githubv4.Client, error)

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

// AddCommentToPendingReview creates a tool to add a review comment to a pending review
func AddCommentToPendingReview(getClient GetClientFn, getGQLClient GetGQLClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "add_comment_to_pending_review"
	description := "Add review comment to the requester's latest pending pull request review."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("body", mcp.Required(), mcp.Description("The text of the review comment")),
			mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file that necessitates a comment")),
			mcp.WithNumber("pull_number", mcp.Required(), mcp.Description("Pull request number")),
			mcp.WithString("subjectType", mcp.Required(), mcp.Description("The level at which the comment is targeted"), mcp.Enum("FILE", "LINE")),
			mcp.WithNumber("line", mcp.Description("The line of the blob in the pull request diff that the comment applies to. For multi-line comments, the last line of the range")),
			mcp.WithString("side", mcp.Description("The side of the diff to comment on. LEFT indicates the previous state, RIGHT indicates the new state"), mcp.Enum("LEFT", "RIGHT")),
			mcp.WithNumber("startLine", mcp.Description("For multi-line comments, the first line of the range that the comment applies to")),
			mcp.WithString("startSide", mcp.Description("For multi-line comments, the starting side of the diff that the comment applies to. LEFT indicates the previous state, RIGHT indicates the new state"), mcp.Enum("LEFT", "RIGHT")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				PullNumber  int32   `mapstructure:"pull_number"`
				Path        string  `mapstructure:"path"`
				Body        string  `mapstructure:"body"`
				SubjectType string  `mapstructure:"subjectType"`
				Line        *int32  `mapstructure:"line"`
				Side        *string `mapstructure:"side"`
				StartLine   *int32  `mapstructure:"startLine"`
				StartSide   *string `mapstructure:"startSide"`
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			restClient, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub REST client: %w", err)
			}

			// Adjust suggestion indentation if a suggestion block exists
			var adjustedBody string
			if params.Line != nil {
				adjustedBody, err = adjustSuggestionIndentation(ctx, restClient, owner, repo, int(params.PullNumber), params.Path, int(*params.Line), params.Body)
				if err != nil {
					// If adjustment fails, log the error and proceed with the original body
					// This ensures that the comment is still added even if indentation adjustment fails
					fmt.Fprintf(os.Stderr, "Failed to adjust suggestion indentation: %v\n", err)
					adjustedBody = params.Body
				}
			} else {
				// No line specified, use original body without adjustment
				adjustedBody = params.Body
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub GQL client: %w", err)
			}

			// First we'll get the current user
			var getViewerQuery struct {
				Viewer struct {
					Login githubv4.String
				}
			}

			if err := client.Query(ctx, &getViewerQuery, nil); err != nil {
				return NewGitHubGraphQLErrorResponse(ctx,
					"failed to get current user",
					err,
				), nil
			}

			var getLatestReviewForViewerQuery struct {
				Repository struct {
					PullRequest struct {
						Reviews struct {
							Nodes []struct {
								ID    githubv4.ID
								State githubv4.PullRequestReviewState
								URL   githubv4.URI
							}
						} `graphql:"reviews(first: 1, author: $author, states: PENDING)"`
					} `graphql:"pullRequest(number: $prNum)"`
				} `graphql:"repository(owner: $owner, name: $name)"`
			}

			vars := map[string]any{
				"author": githubv4.String(getViewerQuery.Viewer.Login),
				"owner":  githubv4.String(owner),
				"name":   githubv4.String(repo),
				"prNum":  githubv4.Int(params.PullNumber),
			}

			if err := client.Query(context.Background(), &getLatestReviewForViewerQuery, vars); err != nil {
				return NewGitHubGraphQLErrorResponse(ctx,
					"failed to get latest review for current user",
					err,
				), nil
			}

			// Validate there is one review and the state is pending
			if len(getLatestReviewForViewerQuery.Repository.PullRequest.Reviews.Nodes) == 0 {
				return mcp.NewToolResultError("No pending review found for the viewer"), nil
			}

			review := getLatestReviewForViewerQuery.Repository.PullRequest.Reviews.Nodes[0]
			if review.State != githubv4.PullRequestReviewStatePending {
				errText := fmt.Sprintf("The latest review, found at %s is not pending", review.URL)
				return mcp.NewToolResultError(errText), nil
			}

			// Create QoderFixContext for the footer link
			fixContext := QoderFixContext{
				Owner:      owner,
				Repo:       repo,
				PullNumber: int(params.PullNumber),
				Path:       params.Path,
				Body:       adjustedBody, // Use adjusted body here
			}

			if params.Line != nil {
				fixContext.Line = int(*params.Line)
			}
			if params.Side != nil {
				fixContext.Side = *params.Side
			}
			if params.StartLine != nil {
				fixContext.StartLine = int(*params.StartLine)
			}
			if params.StartSide != nil {
				fixContext.StartSide = *params.StartSide
			}

			// Marshal and encode the context for the footer
			// contextJSON, err := json.Marshal(fixContext)
			// if err != nil {
			// 	return mcp.NewToolResultError(fmt.Sprintf("failed to marshal context: %v", err)), nil
			// }
			// encodedContext := base64.StdEncoding.EncodeToString(contextJSON)
			// 			footer := fmt.Sprintf(`

			// ---
			// *Powered by Qoder* | [One-Click Qoder Fix](http://localhost:9080/reload-to-qoder?context=%s)`, encodedContext)
			footer := `

---
*Powered by Qoder*`
			fullBody := adjustedBody + footer // Use adjusted body here as well

			// Then we can create a new review thread comment on the review.
			var addPullRequestReviewThreadMutation struct {
				AddPullRequestReviewThread struct {
					Thread struct {
						ID githubv4.ID // We don't need this, but a selector is required or GQL complains.
					}
				} `graphql:"addPullRequestReviewThread(input: $input)"`
			}

			if err := client.Mutate(
				ctx,
				&addPullRequestReviewThreadMutation,
				githubv4.AddPullRequestReviewThreadInput{
					Path:                githubv4.String(params.Path),
					Body:                githubv4.String(fullBody),
					SubjectType:         newGQLStringlikePtr[githubv4.PullRequestReviewThreadSubjectType](&params.SubjectType),
					Line:                newGQLIntPtr(params.Line),
					Side:                newGQLStringlikePtr[githubv4.DiffSide](params.Side),
					StartLine:           newGQLIntPtr(params.StartLine),
					StartSide:           newGQLStringlikePtr[githubv4.DiffSide](params.StartSide),
					PullRequestReviewID: &review.ID,
				},
				nil,
			); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Return nothing interesting, just indicate success for the time being.
			// In future, we may want to return the review ID, but for the moment, we're not leaking
			// API implementation details to the LLM.
			return mcp.NewToolResultText("pull request review comment successfully added to pending review"), nil
		}
}

// SubmitPendingPullRequestReview creates a tool to submit a pending pull request review
func SubmitPendingPullRequestReview(getClient GetClientFn, getGQLClient GetGQLClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "submit_pending_pull_request_review"
	description := "Submit the requester's latest pending pull request review with a specific event type (APPROVE, REQUEST_CHANGES, or COMMENT)"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number", mcp.Required(), mcp.Description("Pull request number")),
			mcp.WithString("event", mcp.Required(), mcp.Description("Review action: APPROVE, REQUEST_CHANGES, or COMMENT"), mcp.Enum("APPROVE", "REQUEST_CHANGES", "COMMENT")),
			mcp.WithString("body", mcp.Description("Summary comment for the review (optional)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				PullNumber int32   `mapstructure:"pull_number"`
				Event      string  `mapstructure:"event"`
				Body       *string `mapstructure:"body"`
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			gqlClient, err := getGQLClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub GQL client: %w", err)
			}

			// Get the current user
			var getViewerQuery struct {
				Viewer struct {
					Login githubv4.String
				}
			}

			if err := gqlClient.Query(ctx, &getViewerQuery, nil); err != nil {
				return NewGitHubGraphQLErrorResponse(ctx,
					"failed to get current user",
					err,
				), nil
			}

			// Get the latest pending review for the current user
			var getLatestReviewQuery struct {
				Repository struct {
					PullRequest struct {
						Reviews struct {
							Nodes []struct {
								DatabaseID int64
								State      githubv4.PullRequestReviewState
								URL        githubv4.URI
							}
						} `graphql:"reviews(first: 1, author: $author, states: PENDING)"`
					} `graphql:"pullRequest(number: $prNum)"`
				} `graphql:"repository(owner: $owner, name: $name)"`
			}

			vars := map[string]any{
				"author": githubv4.String(getViewerQuery.Viewer.Login),
				"owner":  githubv4.String(owner),
				"name":   githubv4.String(repo),
				"prNum":  githubv4.Int(params.PullNumber),
			}

			if err := gqlClient.Query(ctx, &getLatestReviewQuery, vars); err != nil {
				return NewGitHubGraphQLErrorResponse(ctx,
					"failed to get pending review",
					err,
				), nil
			}

			// Validate there is a pending review
			if len(getLatestReviewQuery.Repository.PullRequest.Reviews.Nodes) == 0 {
				return mcp.NewToolResultError("No pending review found for the current user"), nil
			}

			review := getLatestReviewQuery.Repository.PullRequest.Reviews.Nodes[0]
			reviewID := review.DatabaseID

			// Use REST API to submit the review (GraphQL SubmitPullRequestReview is not available in go-github)
			restClient, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub REST client: %w", err)
			}

			// Submit the review
			submitRequest := &github.PullRequestReviewRequest{
				Event: github.Ptr(params.Event),
			}
			if params.Body != nil && *params.Body != "" {
				submitRequest.Body = params.Body
			}

			submittedReview, _, err := restClient.PullRequests.SubmitReview(ctx, owner, repo, int(params.PullNumber), reviewID, submitRequest)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to submit review: %v", err)), nil
			}

			// Return the submitted review as JSON
			result, err := json.Marshal(submittedReview)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(result)), nil
		}
}

// CreatePendingPullRequestReview creates a tool to create a new pending pull request review
func CreatePendingPullRequestReview(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "create_pending_pull_request_review"
	description := "Create a new pending pull request review."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number", mcp.Required(), mcp.Description("Pull request number")),
			mcp.WithString("commitId", mcp.Description("The SHA of the commit to review. If not provided, defaults to the most recent commit in the pull request")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				PullNumber int32   `mapstructure:"pull_number"`
				CommitId   *string `mapstructure:"commitId"`
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Create the review request (without Event field for pending review)
			reviewRequest := &github.PullRequestReviewRequest{}

			// Add commit ID if provided
			if params.CommitId != nil && *params.CommitId != "" {
				reviewRequest.CommitID = params.CommitId
			}

			// Create the review
			review, _, err := client.PullRequests.CreateReview(ctx, owner, repo, int(params.PullNumber), reviewRequest)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create pending review: %v", err)), nil
			}

			// Return the created review as JSON
			result, err := json.Marshal(review)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(result)), nil
		}
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

func newGQLStringlike[T ~string](s string) *T {
	if s == "" {
		return nil
	}
	stringlike := T(s)
	return &stringlike
}

func newGQLStringlikePtr[T ~string](s *string) *T {
	if s == nil {
		return nil
	}
	stringlike := T(*s)
	return &stringlike
}

func newGQLIntPtr(i *int32) *githubv4.Int {
	if i == nil {
		return nil
	}
	gi := githubv4.Int(*i)
	return &gi
}

func NewGitHubGraphQLErrorResponse(ctx context.Context, message string, err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf("%s: %v", message, err))
}

// ReplyComment creates a tool to reply to an existing comment (issue or review)
func ReplyComment(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "reply_comment"
	description := "Reply to an existing GitHub comment (issue comment or review comment)"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("comment_type",
				mcp.Required(),
				mcp.Description("Type of comment to reply to: 'issue' or 'review'"),
				mcp.Enum("issue", "review"),
			),
			mcp.WithNumber("comment_id",
				mcp.Required(),
				mcp.Description("ID of the comment to reply to"),
			),
			mcp.WithString("body",
				mcp.Required(),
				mcp.Description("Reply content"),
			),
			mcp.WithNumber("issue_number",
				mcp.Description("Issue number (required when comment_type is 'issue')"),
			),
			mcp.WithNumber("pull_number",
				mcp.Description("Pull request number (required when comment_type is 'review')"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters
			commentType, err := getRequiredStringParam(request, "comment_type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			commentID, err := getRequiredNumberParam(request, "comment_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			body, err := getRequiredStringParam(request, "body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Handle different comment types
			switch commentType {
			case "issue":
				issueNumber := getOptionalNumberParam(request, "issue_number")
				if issueNumber == 0 {
					return mcp.NewToolResultError("issue_number is required when comment_type is 'issue'"), nil
				}
				return replyToIssueComment(ctx, client, owner, repo, issueNumber, body)
			case "review":
				pullNumber := getOptionalNumberParam(request, "pull_number")
				if pullNumber == 0 {
					return mcp.NewToolResultError("pull_number is required when comment_type is 'review'"), nil
				}
				return replyToReviewComment(ctx, client, owner, repo, pullNumber, int64(commentID), body)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("unsupported comment type: %s", commentType)), nil
			}
		}
}

// UpdateComment creates a tool to update an existing comment's full content
func UpdateComment(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "update_comment"
	description := "Update an existing GitHub comment (issue comment or review comment) with new content"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("comment_type",
				mcp.Required(),
				mcp.Description("Type of comment to update: 'issue' or 'review'"),
				mcp.Enum("issue", "review"),
			),
			mcp.WithNumber("comment_id",
				mcp.Required(),
				mcp.Description("ID of the comment to update"),
			),
			mcp.WithString("body",
				mcp.Required(),
				mcp.Description("New content for the comment"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters
			commentType, err := getRequiredStringParam(request, "comment_type")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			commentID, err := getRequiredNumberParam(request, "comment_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			body, err := getRequiredStringParam(request, "body")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Handle different comment types
			switch commentType {
			case "issue":
				return updateFullIssueComment(ctx, client, owner, repo, int64(commentID), body)
			case "review":
				return updateFullReviewComment(ctx, client, owner, repo, int64(commentID), body)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("unsupported comment type: %s", commentType)), nil
			}
		}
}

// replyToIssueComment creates a new comment on an issue as a reply
func replyToIssueComment(ctx context.Context, client *github.Client, owner, repo string, issueNumber int, body string) (*mcp.CallToolResult, error) {
	// Create a new comment on the issue
	comment := &github.IssueComment{
		Body: github.Ptr(body),
	}

	createdComment, _, err := client.Issues.CreateComment(ctx, owner, repo, issueNumber, comment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create issue comment: %v", err)), nil
	}

	// Return the created comment as JSON
	result, err := json.Marshal(createdComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// replyToReviewComment creates a reply to a pull request review comment
// Always replies to the root (top-level) comment to avoid deep nesting
func replyToReviewComment(ctx context.Context, client *github.Client, owner, repo string, pullNumber int, commentID int64, body string) (*mcp.CallToolResult, error) {
	// Find the root comment to avoid deep nesting
	rootCommentID, err := findRootReviewComment(ctx, client, owner, repo, commentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to find root comment: %v", err)), nil
	}

	// Create a reply to the root comment
	replyComment, _, err := client.PullRequests.CreateCommentInReplyTo(ctx, owner, repo, pullNumber, body, rootCommentID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to create review comment reply: %v", err)), nil
	}

	// Return the created reply as JSON
	result, err := json.Marshal(replyComment)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

// updateFullIssueComment updates the full content of an issue comment
func updateFullIssueComment(ctx context.Context, client *github.Client, owner, repo string, commentID int64, newBody string) (*mcp.CallToolResult, error) {
	// Update the comment with new content
	updateComment := &github.IssueComment{
		Body: github.Ptr(newBody),
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

// updateFullReviewComment updates the full content of a review comment
func updateFullReviewComment(ctx context.Context, client *github.Client, owner, repo string, commentID int64, newBody string) (*mcp.CallToolResult, error) {
	// Update the comment with new content
	updateComment := &github.PullRequestComment{
		Body: github.Ptr(newBody),
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

// findRootReviewComment finds the root (top-level) comment of a review comment thread
func findRootReviewComment(ctx context.Context, client *github.Client, owner, repo string, commentID int64) (int64, error) {
	currentID := commentID
	visited := make(map[int64]bool) // Prevent infinite loops

	for {
		// Prevent infinite loop in case of circular references
		if visited[currentID] {
			return 0, fmt.Errorf("circular reference detected in comment thread")
		}
		visited[currentID] = true

		// Get the comment details
		comment, _, err := client.PullRequests.GetComment(ctx, owner, repo, currentID)
		if err != nil {
			return 0, fmt.Errorf("failed to get comment %d: %w", currentID, err)
		}

		// If this comment has no parent (in_reply_to_id is nil), it's the root
		if comment.InReplyTo == nil || *comment.InReplyTo == 0 {
			return currentID, nil
		}

		// Move to the parent comment
		currentID = *comment.InReplyTo
	}
}

// GetPullRequestDiff creates a tool to get PR diff with enhanced line numbers and compression
func GetPullRequestDiff(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pull_request_diff"
	description := "Get pull request diff with line numbers showing the latest file state. New lines and context lines show their line numbers, deleted lines don't. Automatically applies compression strategies to reduce diff size when needed."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number",
				mcp.Required(),
				mcp.Description("Pull request number"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters
			pullNumber, err := getRequiredNumberParam(request, "pull_number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Get raw diff from GitHub API
			rawDiff, _, err := client.PullRequests.GetRaw(
				ctx,
				owner,
				repo,
				pullNumber,
				github.RawOptions{Type: github.Diff},
			)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get PR diff: %v", err)), nil
			}

			// Add line numbers to the diff to show the latest file state
			enhancedDiff, err := addLineNumbersToNewLines(string(rawDiff))
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to enhance diff: %v", err)), nil
			}

			// Check if compression is enabled (default: true)
			compressEnabled := true
			if envVal := os.Getenv("PR_DIFF_COMPRESS_ENABLED"); envVal == "false" {
				compressEnabled = false
			}

			// Apply compression if enabled
			if compressEnabled {
				compressor := NewDiffCompressor()
				compressedDiff, err := compressor.CompressDiff(enhancedDiff)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to compress diff: %v", err)), nil
				}
				return mcp.NewToolResultText(compressedDiff), nil
			}

			// Return the enhanced diff without compression
			return mcp.NewToolResultText(enhancedDiff), nil
		}
}

// addLineNumbersToNewLines adds line numbers to new lines and context lines in diff
func addLineNumbersToNewLines(diffContent string) (string, error) {
	lines := strings.Split(diffContent, "\n")
	var result []string
	newLineNum := 0
	inChunk := false

	for _, line := range lines {
		// Check if this is a chunk header
		if strings.HasPrefix(line, "@@") {
			// Parse the chunk header to get starting line numbers
			_, _, newStart, _, err := parseChunkHeader(line)
			if err != nil {
				// If parsing fails, just use the line as-is
				result = append(result, line)
				continue
			}
			newLineNum = newStart
			inChunk = true
			result = append(result, line)
			continue
		}

		// If we're not in a chunk yet, just add the line as-is
		if !inChunk {
			result = append(result, line)
			continue
		}

		// Process lines within chunks
		if len(line) == 0 {
			// Empty line - treat as context
			newLineNum++
			result = append(result, line)
		} else if line[0] == '+' {
			// Added line - format: "lineNumber +content"
			enhancedLine := fmt.Sprintf("%d +%s", newLineNum, line[1:])
			newLineNum++
			result = append(result, enhancedLine)
		} else if line[0] == '-' {
			// Removed line - format: "   -content" (no line number)
			enhancedLine := fmt.Sprintf("   -%s", line[1:])
			result = append(result, enhancedLine)
		} else if line[0] == ' ' {
			// Context line - format: "lineNumber  content" (with line number, no +/-)
			enhancedLine := fmt.Sprintf("%d  %s", newLineNum, line[1:])
			newLineNum++
			result = append(result, enhancedLine)
		} else {
			// Other lines (like file headers) - reset chunk state
			if strings.HasPrefix(line, "diff --git") {
				inChunk = false
			}
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n"), nil
}

// parseChunkHeader parses a chunk header like "@@ -1,4 +1,6 @@"
func parseChunkHeader(header string) (oldStart, oldLines, newStart, newLines int, err error) {
	// Remove @@ at the beginning and end
	header = strings.Trim(header, "@ ")
	parts := strings.Fields(header)
	if len(parts) < 2 {
		return 0, 0, 0, 0, fmt.Errorf("invalid chunk header format")
	}

	// Parse old range: -start,count
	oldPart := parts[0]
	if !strings.HasPrefix(oldPart, "-") {
		return 0, 0, 0, 0, fmt.Errorf("invalid old range format")
	}
	oldPart = oldPart[1:] // Remove -
	if strings.Contains(oldPart, ",") {
		oldRange := strings.Split(oldPart, ",")
		oldStart, err = strconv.Atoi(oldRange[0])
		if err != nil {
			return 0, 0, 0, 0, err
		}
		oldLines, err = strconv.Atoi(oldRange[1])
		if err != nil {
			return 0, 0, 0, 0, err
		}
	} else {
		oldStart, err = strconv.Atoi(oldPart)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		oldLines = 1
	}

	// Parse new range: +start,count
	newPart := parts[1]
	if !strings.HasPrefix(newPart, "+") {
		return 0, 0, 0, 0, fmt.Errorf("invalid new range format")
	}
	newPart = newPart[1:] // Remove +
	if strings.Contains(newPart, ",") {
		newRange := strings.Split(newPart, ",")
		newStart, err = strconv.Atoi(newRange[0])
		if err != nil {
			return 0, 0, 0, 0, err
		}
		newLines, err = strconv.Atoi(newRange[1])
		if err != nil {
			return 0, 0, 0, 0, err
		}
	} else {
		newStart, err = strconv.Atoi(newPart)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		newLines = 1
	}

	return oldStart, oldLines, newStart, newLines, nil
}

// GetPullRequestFiles creates a tool to get PR files with enhanced patch content
func GetPullRequestFiles(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pull_request_files"
	description := "Get the files changed in a pull request. Returns file metadata and enhanced patch content with line numbers. Automatically compresses patch content when needed to avoid context overflow."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number",
				mcp.Required(),
				mcp.Description("Pull request number"),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (default: 1)"),
			),
			mcp.WithNumber("per_page",
				mcp.Description("Number of items per page (default: 30, max: 100)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			pullNumber, err := getRequiredNumberParam(request, "pull_number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Extract optional pagination parameters
			page := getOptionalNumberParam(request, "page")
			if page == 0 {
				page = 1
			}

			perPage := getOptionalNumberParam(request, "per_page")
			if perPage == 0 {
				perPage = 30
			}
			if perPage > 100 {
				perPage = 100
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Fetch pull request files from GitHub API
			opts := &github.ListOptions{
				Page:    page,
				PerPage: perPage,
			}

			files, resp, err := client.PullRequests.ListFiles(ctx, owner, repo, pullNumber, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get PR files: %v", err)), nil
			}

			// Enhance patch content with line numbers
			for _, file := range files {
				if file.Patch != nil && *file.Patch != "" {
					enhancedPatch, err := addLineNumbersToNewLines(*file.Patch)
					if err != nil {
						return mcp.NewToolResultError(fmt.Sprintf("failed to enhance patch for file %s: %v", file.GetFilename(), err)), nil
					}
					file.Patch = &enhancedPatch
				}
			}

			// Apply compression if enabled
			compressEnabled := true
			if envVal := os.Getenv("PR_DIFF_COMPRESS_ENABLED"); envVal == "false" {
				compressEnabled = false
			}

			if compressEnabled {
				fileCompressor := NewFileListCompressor()
				files = fileCompressor.CompressFileList(files)
			}

			// Create response structure with pagination info
			result := struct {
				Files      []*github.CommitFile `json:"files"`
				Page       int                  `json:"page"`
				PerPage    int                  `json:"per_page"`
				HasNext    bool                 `json:"has_next"`
				TotalCount int                  `json:"total_count"`
			}{
				Files:      files,
				Page:       page,
				PerPage:    perPage,
				HasNext:    resp.NextPage > 0,
				TotalCount: len(files),
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// GetPullRequest creates a tool to get pull request details
func GetPullRequest(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pull_request"
	description := "Get detailed information about a pull request, including title, body, state, author, reviewers, labels, and more."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number",
				mcp.Required(),
				mcp.Description("Pull request number"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract parameters
			pullNumber, err := getRequiredNumberParam(request, "pull_number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Get pull request details from GitHub API
			pr, _, err := client.PullRequests.Get(ctx, owner, repo, pullNumber)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get PR: %v", err)), nil
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(pr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal PR: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// GetPullRequestComments creates a tool to get all comments on a pull request
func GetPullRequestComments(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pull_request_comments"
	description := "Get all review comments on a pull request. These are inline comments on specific lines of code in the diff."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number",
				mcp.Required(),
				mcp.Description("Pull request number"),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (default: 1)"),
			),
			mcp.WithNumber("per_page",
				mcp.Description("Number of items per page (default: 30, max: 100)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			pullNumber, err := getRequiredNumberParam(request, "pull_number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Extract optional pagination parameters
			page := getOptionalNumberParam(request, "page")
			if page == 0 {
				page = 1
			}

			perPage := getOptionalNumberParam(request, "per_page")
			if perPage == 0 {
				perPage = 30
			}
			if perPage > 100 {
				perPage = 100
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Fetch review comments from GitHub API
			opts := &github.PullRequestListCommentsOptions{
				ListOptions: github.ListOptions{
					Page:    page,
					PerPage: perPage,
				},
			}

			comments, resp, err := client.PullRequests.ListComments(ctx, owner, repo, pullNumber, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get PR comments: %v", err)), nil
			}

			// Create response structure with pagination info
			result := struct {
				Comments   []*github.PullRequestComment `json:"comments"`
				Page       int                          `json:"page"`
				PerPage    int                          `json:"per_page"`
				HasNext    bool                         `json:"has_next"`
				TotalCount int                          `json:"total_count"`
			}{
				Comments:   comments,
				Page:       page,
				PerPage:    perPage,
				HasNext:    resp.NextPage > 0,
				TotalCount: len(comments),
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal comments: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// GetPullRequestReviews creates a tool to get all reviews on a pull request
func GetPullRequestReviews(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pull_request_reviews"
	description := "Get all reviews on a pull request, including review state (APPROVED, CHANGES_REQUESTED, COMMENTED), reviewer, and review body."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithNumber("pull_number",
				mcp.Required(),
				mcp.Description("Pull request number"),
			),
			mcp.WithNumber("page",
				mcp.Description("Page number for pagination (default: 1)"),
			),
			mcp.WithNumber("per_page",
				mcp.Description("Number of items per page (default: 30, max: 100)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			pullNumber, err := getRequiredNumberParam(request, "pull_number")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Extract optional pagination parameters
			page := getOptionalNumberParam(request, "page")
			if page == 0 {
				page = 1
			}

			perPage := getOptionalNumberParam(request, "per_page")
			if perPage == 0 {
				perPage = 30
			}
			if perPage > 100 {
				perPage = 100
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Fetch reviews from GitHub API
			opts := &github.ListOptions{
				Page:    page,
				PerPage: perPage,
			}

			reviews, resp, err := client.PullRequests.ListReviews(ctx, owner, repo, pullNumber, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get PR reviews: %v", err)), nil
			}

			// Create response structure with pagination info
			result := struct {
				Reviews    []*github.PullRequestReview `json:"reviews"`
				Page       int                         `json:"page"`
				PerPage    int                         `json:"per_page"`
				HasNext    bool                        `json:"has_next"`
				TotalCount int                         `json:"total_count"`
			}{
				Reviews:    reviews,
				Page:       page,
				PerPage:    perPage,
				HasNext:    resp.NextPage > 0,
				TotalCount: len(reviews),
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal reviews: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// CreateOrUpdateFile creates a tool to create or update a single file in a GitHub repository
func CreateOrUpdateFile(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "create_or_update_file"
	description := "Create or update a single file in a GitHub repository. If updating, you must provide the SHA of the file you want to update."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path where to create/update the file"),
			),
			mcp.WithString("content",
				mcp.Required(),
				mcp.Description("Content of the file"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to create/update the file in"),
			),
			mcp.WithString("sha",
				mcp.Description("Required if updating an existing file. The blob SHA of the file being replaced."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			path, err := getRequiredStringParam(request, "path")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			content, err := getRequiredStringParam(request, "content")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			message, err := getRequiredStringParam(request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			branch, err := getRequiredStringParam(request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get optional SHA parameter
			sha := request.GetString("sha", "")

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Create the file options
			contentBytes := []byte(content)
			opts := &github.RepositoryContentFileOptions{
				Message: github.Ptr(message),
				Content: contentBytes,
				Branch:  github.Ptr(branch),
			}

			// If SHA is provided, set it (for updates)
			if sha != "" {
				opts.SHA = github.Ptr(sha)
			}

			// Create or update the file
			fileContent, _, err := client.Repositories.CreateFile(ctx, owner, repo, path, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create/update file: %v", err)), nil
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(fileContent)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// PushFiles creates a tool to push multiple files in a single commit to a GitHub repository
func PushFiles(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "push_files"
	description := "Push multiple files to a GitHub repository in a single commit"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Branch to push to"),
			),
			mcp.WithArray("files",
				mcp.Required(),
				mcp.Items(
					map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"path", "content"},
						"properties": map[string]interface{}{
							"path": map[string]interface{}{
								"type":        "string",
								"description": "path to the file",
							},
							"content": map[string]interface{}{
								"type":        "string",
								"description": "file content",
							},
						},
					}),
				mcp.Description("Array of file objects to push, each object with path (string) and content (string)"),
			),
			mcp.WithString("message",
				mcp.Required(),
				mcp.Description("Commit message"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			branch, err := getRequiredStringParam(request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			message, err := getRequiredStringParam(request, "message")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse files parameter
			filesObj, ok := request.GetArguments()["files"].([]interface{})
			if !ok {
				return mcp.NewToolResultError("files parameter must be an array of objects with path and content"), nil
			}

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Get the reference for the branch
			ref, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get branch reference: %v", err)), nil
			}

			// Get the commit object that the branch points to
			baseCommit, _, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get base commit: %v", err)), nil
			}

			// Create tree entries for all files
			var entries []*github.TreeEntry

			for _, file := range filesObj {
				fileMap, ok := file.(map[string]interface{})
				if !ok {
					return mcp.NewToolResultError("each file must be an object with path and content"), nil
				}

				path, ok := fileMap["path"].(string)
				if !ok || path == "" {
					return mcp.NewToolResultError("each file must have a path"), nil
				}

				content, ok := fileMap["content"].(string)
				if !ok {
					return mcp.NewToolResultError("each file must have content"), nil
				}

				// Create a tree entry for the file
				entries = append(entries, &github.TreeEntry{
					Path:    github.Ptr(path),
					Mode:    github.Ptr("100644"), // Regular file mode
					Type:    github.Ptr("blob"),
					Content: github.Ptr(content),
				})
			}

			// Create a new tree with the file entries
			newTree, _, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, entries)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create tree: %v", err)), nil
			}

			// Create a new commit
			commit := &github.Commit{
				Message: github.Ptr(message),
				Tree:    newTree,
				Parents: []*github.Commit{{SHA: baseCommit.SHA}},
			}
			newCommit, _, err := client.Git.CreateCommit(ctx, owner, repo, commit, nil)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create commit: %v", err)), nil
			}

			// Update the reference to point to the new commit
			ref.Object.SHA = newCommit.SHA
			updatedRef, _, err := client.Git.UpdateRef(ctx, owner, repo, ref, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to update reference: %v", err)), nil
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(updatedRef)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// CreateBranch creates a tool to create a new branch in a GitHub repository
func CreateBranch(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "create_branch"
	description := "Create a new branch in a GitHub repository"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("branch",
				mcp.Required(),
				mcp.Description("Name for new branch"),
			),
			mcp.WithString("from_branch",
				mcp.Description("Source branch (defaults to repo default branch)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			branch, err := getRequiredStringParam(request, "branch")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get optional from_branch parameter
			fromBranch := request.GetString("from_branch", "")

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Get the source branch SHA
			var ref *github.Reference

			if fromBranch == "" {
				// Get default branch if from_branch not specified
				repository, _, err := client.Repositories.Get(ctx, owner, repo)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to get repository: %v", err)), nil
				}
				fromBranch = *repository.DefaultBranch
			}

			// Get SHA of source branch
			ref, _, err = client.Git.GetRef(ctx, owner, repo, "refs/heads/"+fromBranch)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get reference: %v", err)), nil
			}

			// Create new branch
			newRef := &github.Reference{
				Ref:    github.Ptr("refs/heads/" + branch),
				Object: &github.GitObject{SHA: ref.Object.SHA},
			}

			createdRef, _, err := client.Git.CreateRef(ctx, owner, repo, newRef)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create branch: %v", err)), nil
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(createdRef)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}

// CreatePullRequest creates a tool to create a new pull request
func CreatePullRequest(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "create_pull_request"
	description := "Create a new pull request in a GitHub repository"

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("PR title"),
			),
			mcp.WithString("head",
				mcp.Required(),
				mcp.Description("Branch containing changes"),
			),
			mcp.WithString("base",
				mcp.Required(),
				mcp.Description("Branch to merge into"),
			),
			mcp.WithString("body",
				mcp.Description("PR description"),
			),
			mcp.WithBoolean("draft",
				mcp.Description("Create as draft PR (default: false)"),
			),
			mcp.WithBoolean("maintainer_can_modify",
				mcp.Description("Allow maintainer edits (default: false)"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Extract required parameters
			title, err := getRequiredStringParam(request, "title")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			head, err := getRequiredStringParam(request, "head")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			base, err := getRequiredStringParam(request, "base")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Get optional parameters
			body := request.GetString("body", "")
			draft := request.GetBool("draft", false)
			maintainerCanModify := request.GetBool("maintainer_can_modify", false)

			// Get GitHub client
			client, err := getClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
			}

			// Create the pull request
			newPR := &github.NewPullRequest{
				Title:               github.Ptr(title),
				Head:                github.Ptr(head),
				Base:                github.Ptr(base),
				Draft:               github.Ptr(draft),
				MaintainerCanModify: github.Ptr(maintainerCanModify),
			}

			if body != "" {
				newPR.Body = github.Ptr(body)
			}

			pr, _, err := client.PullRequests.Create(ctx, owner, repo, newPR)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to create pull request: %v", err)), nil
			}

			// Marshal to JSON and return
			resultJSON, err := json.Marshal(pr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
			}

			return mcp.NewToolResultText(string(resultJSON)), nil
		}
}
