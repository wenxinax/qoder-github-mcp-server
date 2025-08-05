package qoder

import (
	"testing"
)

func TestReplaceQoderContent(t *testing.T) {
	tests := []struct {
		name           string
		originalBody   string
		newContent     string
		expectedResult string
		expectError    bool
	}{
		{
			name: "successful replacement",
			originalBody: `Some content before
<!-- QODER_BODY_START -->
Old content here
<!-- QODER_BODY_END -->
Some content after`,
			newContent: "New content from Qoder",
			expectedResult: `Some content before
<!-- QODER_BODY_START -->
New content from Qoder
<!-- QODER_BODY_END -->
Some content after`,
			expectError: false,
		},
		{
			name: "missing start marker",
			originalBody: `Some content
<!-- QODER_BODY_END -->
More content`,
			newContent:  "New content",
			expectError: true,
		},
		{
			name: "missing end marker",
			originalBody: `Some content
<!-- QODER_BODY_START -->
More content`,
			newContent:  "New content",
			expectError: true,
		},
		{
			name: "end marker before start marker",
			originalBody: `Some content
<!-- QODER_BODY_END -->
<!-- QODER_BODY_START -->
More content`,
			newContent:  "New content",
			expectError: true,
		},
		{
			name: "empty content replacement",
			originalBody: `Before
<!-- QODER_BODY_START -->
Old content
<!-- QODER_BODY_END -->
After`,
			newContent: "",
			expectedResult: `Before
<!-- QODER_BODY_START -->

<!-- QODER_BODY_END -->
After`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replaceQoderContent(tt.originalBody, tt.newContent)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if result != tt.expectedResult {
				t.Errorf("Expected:\n%s\n\nGot:\n%s", tt.expectedResult, result)
			}
		})
	}
}

func TestGetRequiredStringParam(t *testing.T) {
	// Note: This test would require mocking mcp.CallToolRequest
	// For now, we'll just test the replaceQoderContent function
	// In a real implementation, you might want to create a mock for CallToolRequest
	t.Skip("Skipping test that requires mocking CallToolRequest")
}
