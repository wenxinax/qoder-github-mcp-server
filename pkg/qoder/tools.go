package qoder

import (
	"context"
	"encoding/base64"
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

// QoderUpdateComment creates a tool to update a comment (issue or review) with content between Qoder markers
func QoderUpdateComment(getClient GetClientFn, owner, repo, commentID, commentType string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "update_comment"
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
func QoderAddCommentToPendingReview(getClient GetClientFn, getGQLClient GetGQLClientFn) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "add_comment_to_pending_review"
	description := "Add review comment to the requester's latest pending pull request review. It automatically finds the latest commit."

	return mcp.NewTool(toolName,
			mcp.WithDescription(description),
			mcp.WithString("body", mcp.Required(), mcp.Description("The text of the review comment")),
			mcp.WithString("owner", mcp.Required(), mcp.Description("Repository owner")),
			mcp.WithString("path", mcp.Required(), mcp.Description("The relative path to the file that necessitates a comment")),
			mcp.WithNumber("pullNumber", mcp.Required(), mcp.Description("Pull request number")),
			mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
			mcp.WithString("subjectType", mcp.Required(), mcp.Description("The level at which the comment is targeted"), mcp.Enum("FILE", "LINE")),
			mcp.WithNumber("line", mcp.Description("The line of the blob in the pull request diff that the comment applies to. For multi-line comments, the last line of the range")),
			mcp.WithString("side", mcp.Description("The side of the diff to comment on. LEFT indicates the previous state, RIGHT indicates the new state"), mcp.Enum("LEFT", "RIGHT")),
			mcp.WithNumber("startLine", mcp.Description("For multi-line comments, the first line of the range that the comment applies to")),
			mcp.WithString("startSide", mcp.Description("For multi-line comments, the starting side of the diff that the comment applies to. LEFT indicates the previous state, RIGHT indicates the new state"), mcp.Enum("LEFT", "RIGHT")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Owner       string
				Repo        string
				PullNumber  int32
				Path        string
				Body        string
				SubjectType string
				Line        *int32
				Side        *string
				StartLine   *int32
				StartSide   *string
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			restClient, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub REST client: %w", err)
			}

			// Adjust suggestion indentation if a suggestion block exists
			adjustedBody, err := adjustSuggestionIndentation(ctx, restClient, params.Owner, params.Repo, int(params.PullNumber), params.Path, int(*params.Line), params.Body)
			if err != nil {
				// If adjustment fails, log the error and proceed with the original body
				// This ensures that the comment is still added even if indentation adjustment fails
				fmt.Fprintf(os.Stderr, "Failed to adjust suggestion indentation: %v\n", err)
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
						} `graphql:"reviews(first: 1, author: $author)"`
					} `graphql:"pullRequest(number: $prNum)"`
				} `graphql:"repository(owner: $owner, name: $name)"`
			}

			vars := map[string]any{
				"author": githubv4.String(getViewerQuery.Viewer.Login),
				"owner":  githubv4.String(params.Owner),
				"name":   githubv4.String(params.Repo),
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
				Owner:      params.Owner,
				Repo:       params.Repo,
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
			contextJSON, err := json.Marshal(fixContext)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to marshal context: %v", err)), nil
			}
			encodedContext := base64.StdEncoding.EncodeToString(contextJSON)
			footer := fmt.Sprintf(`

---
*Powered by Qoder* | [One-Click Qoder Fix](http://localhost:9080/reload-to-qoder?context=%s)`, encodedContext)
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

// QoderGetPRDiff creates a tool to get PR diff with enhanced line numbers
func QoderGetPRDiff(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pr_diff"
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

// QoderGetPRFiles creates a tool to get PR files with enhanced patch content
func QoderGetPRFiles(getClient GetClientFn, owner, repo string) (mcp.Tool, server.ToolHandlerFunc) {
	toolName := "get_pr_files"
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
