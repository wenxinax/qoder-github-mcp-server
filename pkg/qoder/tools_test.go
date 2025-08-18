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

func TestAddLineNumbersToNewLines(t *testing.T) {
	tests := []struct {
		name           string
		diffContent    string
		expectedOutput string
		expectedErr    bool
	}{
		{
			name: "simple diff with added lines",
			diffContent: `diff --git a/test.txt b/test.txt
index 1234567..abcdefg 100644
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+added line
 line2
 line3`,
			expectedOutput: `diff --git a/test.txt b/test.txt
index 1234567..abcdefg 100644
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
1  line1
2 +added line
3  line2
4  line3`,
			expectedErr: false,
		},
		{
			name: "multiple added lines with line numbers",
			diffContent: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -5,6 +5,9 @@ func main() {
 	println(\"Hello\")
+	println(\"World\")
+	println(\"from\")
 	println(\"Go\")
+	println(\"!\")\n }`,
			expectedOutput: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -5,6 +5,9 @@ func main() {
5  	println(\"Hello\")
6 +	println(\"World\")
7 +	println(\"from\")
8  	println(\"Go\")
9 +	println(\"!\")\n }`,
			expectedErr: false,
		},
		{
			name: "diff with added, removed, and context lines",
			diffContent: `diff --git a/example.go b/example.go
index 1111111..2222222 100644
--- a/example.go
+++ b/example.go
@@ -10,8 +10,9 @@ func example() {
 fmt.Println("start")
-fmt.Println("old line")
+fmt.Println("new line 1")
+fmt.Println("new line 2")
 fmt.Println("end")
 }`,
			expectedOutput: `diff --git a/example.go b/example.go
index 1111111..2222222 100644
--- a/example.go
+++ b/example.go
@@ -10,8 +10,9 @@ func example() {
10  fmt.Println("start")
   -fmt.Println("old line")
11 +fmt.Println("new line 1")
12 +fmt.Println("new line 2")
13  fmt.Println("end")
14  }`,
			expectedErr: false,
		},
		{
			name: "diff with tabs - should preserve tabs",
			diffContent: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 	func main() {
+		println("Hello")
 	}`,
			expectedOutput: `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
1  	func main() {
2 +		println("Hello")
3  	}`,
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := addLineNumbersToNewLines(tt.diffContent)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expectedOutput {
				t.Errorf("Expected:\n%s\n\nGot:\n%s", tt.expectedOutput, result)
			}
		})
	}
}

func TestParseChunkHeader(t *testing.T) {
	tests := []struct {
		name             string
		header           string
		expectedOldStart int
		expectedOldLines int
		expectedNewStart int
		expectedNewLines int
		expectedErr      bool
	}{
		{
			name:             "standard chunk header",
			header:           "@@ -1,3 +1,4 @@",
			expectedOldStart: 1,
			expectedOldLines: 3,
			expectedNewStart: 1,
			expectedNewLines: 4,
			expectedErr:      false,
		},
		{
			name:             "single line chunk",
			header:           "@@ -5 +5,2 @@",
			expectedOldStart: 5,
			expectedOldLines: 1,
			expectedNewStart: 5,
			expectedNewLines: 2,
			expectedErr:      false,
		},
		{
			name:        "invalid format",
			header:      "invalid header",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStart, oldLines, newStart, newLines, err := parseChunkHeader(tt.header)

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if oldStart != tt.expectedOldStart {
				t.Errorf("Expected old start %d, got %d", tt.expectedOldStart, oldStart)
			}
			if oldLines != tt.expectedOldLines {
				t.Errorf("Expected old lines %d, got %d", tt.expectedOldLines, oldLines)
			}
			if newStart != tt.expectedNewStart {
				t.Errorf("Expected new start %d, got %d", tt.expectedNewStart, newStart)
			}
			if newLines != tt.expectedNewLines {
				t.Errorf("Expected new lines %d, got %d", tt.expectedNewLines, newLines)
			}
		})
	}
}
