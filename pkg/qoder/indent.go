package qoder

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v73/github"
)

// ====== Core Indentation Functions ======

// detectBaseIndentation detects the base indentation of a code block
// Returns the indentation of the first non-empty line
func detectBaseIndentation(codeBlock string) string {
	lines := strings.Split(codeBlock, "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue // Skip empty lines
		}

		// Return the indentation of the first non-empty line
		return getIndentation(line)
	}

	// If all lines are empty, return empty string
	return ""
}

// removeBaseIndentation removes the base indentation from a code block
// Each line will have the baseIndentation prefix removed
func removeBaseIndentation(codeBlock string, baseIndentation string) string {
	lines := strings.Split(codeBlock, "\n")
	var result []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			// Keep empty lines as-is
			result = append(result, "")
		} else {
			// Remove base indentation from non-empty lines
			unindentedLine := strings.TrimPrefix(line, baseIndentation)
			result = append(result, unindentedLine)
		}
	}

	return strings.Join(result, "\n")
}

// applyBaseIndentation applies a base indentation to a code block
// Each non-empty line will have the targetIndentation prefix added
func applyBaseIndentation(codeBlock string, targetIndentation string) string {
	lines := strings.Split(codeBlock, "\n")
	var result []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			// Keep empty lines as-is
			result = append(result, "")
		} else {
			// Add target indentation to non-empty lines
			result = append(result, targetIndentation+line)
		}
	}

	return strings.Join(result, "\n")
}

// getIndentation returns the leading whitespace of a string
func getIndentation(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// ====== Main Adjustment Function ======

// adjustSuggestionIndentation adjusts the indentation of a suggestion block in a comment body
func adjustSuggestionIndentation(ctx context.Context, client *github.Client, owner, repo string, pullNumber int, path string, line int, body string) (string, error) {
	suggestion, err := extractSuggestionBlock(body)
	if err != nil {
		// If no suggestion block is found, return the original body
		return body, nil
	}
	suggestion = strings.TrimSpace(suggestion)
	if suggestion == "" {
		return body, nil
	}

	// Get the indentation from the original code
	correctIndentation, err := getOriginalCodeIndentation(ctx, client, owner, repo, pullNumber, path, line)
	if err != nil {
		return "", fmt.Errorf("failed to get original code indentation: %w", err)
	}

	// Use the new independent methods for indentation adjustment
	baseIndentation := detectBaseIndentation(suggestion)
	if baseIndentation == "" {
		// No indentation to adjust
		return body, nil
	}

	// Step 1: Remove base indentation
	unindentedSuggestion := removeBaseIndentation(suggestion, baseIndentation)

	// Step 2: Apply target indentation
	adjustedSuggestion := applyBaseIndentation(unindentedSuggestion, correctIndentation)

	// Re-insert the adjusted suggestion into the original body
	// We need to find the full original suggestion block to replace it correctly
	originalSuggestionBlock, err := getFullSuggestionBlock(body)
	if err != nil {
		return body, nil // Should not happen if extractSuggestionBlock succeeded
	}
	finalSuggestionBlock := "```suggestion\n" + adjustedSuggestion + "\n```"

	return strings.Replace(body, originalSuggestionBlock, finalSuggestionBlock, 1), nil
}

// ====== Helper Functions ======

// getOriginalCodeIndentation gets the indentation of the original code at a specific line
func getOriginalCodeIndentation(ctx context.Context, client *github.Client, owner, repo string, pullNumber int, path string, line int) (string, error) {
	if line < 1 {
		return "", fmt.Errorf("invalid line number %d, must be >= 1", line)
	}

	pr, _, err := client.PullRequests.Get(ctx, owner, repo, pullNumber)
	if err != nil {
		return "", fmt.Errorf("failed to get pull request: %w", err)
	}

	if pr.GetHead() == nil || pr.GetHead().GetSHA() == "" {
		return "", fmt.Errorf("pull request head commit SHA is empty")
	}

	commitSHA := pr.GetHead().GetSHA()

	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: commitSHA})
	if err != nil {
		return "", fmt.Errorf("failed to get file content: %w", err)
	}

	if fileContent == nil {
		return "", fmt.Errorf("file content is nil")
	}

	content, err := fileContent.GetContent()
	if err != nil {
		return "", fmt.Errorf("failed to decode file content: %w", err)
	}

	lines := strings.Split(content, "\n")
	if line > len(lines) {
		return "", fmt.Errorf("line number %d is out of range for file %s (file has %d lines)", line, path, len(lines))
	}

	// Lines are 1-indexed, arrays are 0-indexed
	targetLine := lines[line-1]
	return getIndentation(targetLine), nil
}

// extractSuggestionBlock extracts the suggestion block from a comment body
func extractSuggestionBlock(body string) (string, error) {
	const suggestionStart = "```suggestion"
	const suggestionEnd = "```"

	startIndex := strings.Index(body, suggestionStart)
	if startIndex == -1 {
		return "", fmt.Errorf("no suggestion block found")
	}

	startIndex += len(suggestionStart)
	// Handle optional newline after suggestion start
	if startIndex < len(body) && body[startIndex] == '\n' {
		startIndex++
	}

	// Look for the end marker starting from after the start marker
	endIndex := strings.Index(body[startIndex:], suggestionEnd)
	if endIndex == -1 {
		return "", fmt.Errorf("suggestion block not properly terminated")
	}

	// Make sure we don't go out of bounds
	actualEndIndex := startIndex + endIndex
	if actualEndIndex > len(body) {
		return "", fmt.Errorf("suggestion block end index out of bounds")
	}

	return body[startIndex:actualEndIndex], nil
}

// getFullSuggestionBlock extracts the full suggestion block from a comment body
func getFullSuggestionBlock(body string) (string, error) {
	const suggestionStart = "```suggestion"
	const suggestionEnd = "```"

	startIndex := strings.Index(body, suggestionStart)
	if startIndex == -1 {
		return "", fmt.Errorf("no suggestion block found")
	}

	// Look for the end marker starting from after the start marker
	searchStart := startIndex + len(suggestionStart)
	endIndex := strings.Index(body[searchStart:], suggestionEnd)
	if endIndex == -1 {
		return "", fmt.Errorf("suggestion block not properly terminated")
	}

	// Calculate the actual end position including the end marker
	actualEndIndex := searchStart + endIndex + len(suggestionEnd)
	if actualEndIndex > len(body) {
		return "", fmt.Errorf("suggestion block end index out of bounds")
	}

	return body[startIndex:actualEndIndex], nil
}
