package qoder

import (
	"os"
	"strings"
	"testing"
)

func TestDiffCompressor_FilterSourceCodeFiles(t *testing.T) {
	compressor := NewDiffCompressor()

	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		// Source code files
		{"Go file", "main.go", true},
		{"Python file", "script.py", true},
		{"JavaScript file", "app.js", true},
		{"TypeScript file", "component.ts", true},
		{"Java file", "Main.java", true},
		{"C++ header", "header.hpp", true},

		// Config files
		{"YAML config", "config.yml", true},
		{"JSON config", "package.json", true},
		{"Dockerfile", "Dockerfile", true},
		{"Makefile", "Makefile", true},

		// Non-source files
		{"Image PNG", "logo.png", false},
		{"Image JPG", "photo.jpg", false},
		{"PDF document", "report.pdf", false},
		{"Binary file", "app.exe", false},
		{"Lock file", "package-lock.json", false},
		{"Go sum file", "go.sum", false},

		// README files
		{"README markdown", "README.md", true},
		{"readme lowercase", "readme.txt", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compressor.isSourceCodeFile(tc.path)
			if result != tc.expected {
				t.Errorf("isSourceCodeFile(%s) = %v; want %v", tc.path, result, tc.expected)
			}
		})
	}
}

func TestDiffCompressor_HasOnlyDeletions(t *testing.T) {
	compressor := NewDiffCompressor()

	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "Only deletions",
			content: `diff --git a/file.go b/file.go
@@ -1,3 +1,0 @@
-func old() {
-    return nil
-}`,
			expected: true,
		},
		{
			name: "Only additions",
			content: `diff --git a/file.go b/file.go
@@ -0,0 +1,3 @@
+func new() {
+    return nil
+}`,
			expected: false,
		},
		{
			name: "Mixed changes",
			content: `diff --git a/file.go b/file.go
@@ -1,3 +1,3 @@
-func old() {
+func new() {
     return nil
 }`,
			expected: false,
		},
		{
			name: "With line numbers format",
			content: `diff --git a/file.go b/file.go
@@ -1,3 +1,0 @@
   -func old() {
   -    return nil
   -}`,
			expected: true,
		},
		{
			name: "Context only",
			content: `diff --git a/file.go b/file.go
@@ -1,3 +1,3 @@
 func unchanged() {
     return nil
 }`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compressor.hasOnlyDeletions(tc.content)
			if result != tc.expected {
				t.Errorf("hasOnlyDeletions() = %v; want %v", result, tc.expected)
			}
		})
	}
}

func TestDiffCompressor_CountWords(t *testing.T) {
	compressor := NewDiffCompressor()

	testCases := []struct {
		name     string
		content  string
		expected int
	}{
		{"Empty string", "", 0},
		{"Single word", "hello", 1},
		{"Multiple words", "hello world test", 3},
		{"With newlines", "hello\nworld\ntest", 3},
		{"With tabs", "hello\tworld\ttest", 3},
		{"Mixed whitespace", "  hello   world  \n  test  ", 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compressor.countWords(tc.content)
			if result != tc.expected {
				t.Errorf("countWords(%q) = %d; want %d", tc.content, result, tc.expected)
			}
		})
	}
}

func TestDiffCompressor_ParseDiffIntoFiles(t *testing.T) {
	compressor := NewDiffCompressor()

	diff := `diff --git a/file1.go b/file1.go
index abc123..def456 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,3 @@
-func old() {
+func new() {
     return nil
 }
diff --git a/file2.py b/file2.py
index 111111..222222 100644
--- a/file2.py
+++ b/file2.py
@@ -1,2 +1,3 @@
 def hello():
+    print("world")
     pass`

	files := compressor.parseDiffIntoFiles(diff)

	if len(files) != 2 {
		t.Fatalf("Expected 2 files, got %d", len(files))
	}

	if files[0].Path != "file1.go" {
		t.Errorf("Expected first file path to be 'file1.go', got '%s'", files[0].Path)
	}

	if files[1].Path != "file2.py" {
		t.Errorf("Expected second file path to be 'file2.py', got '%s'", files[1].Path)
	}

	if !files[0].IsSourceCode {
		t.Error("Expected file1.go to be identified as source code")
	}

	if !files[1].IsSourceCode {
		t.Error("Expected file2.py to be identified as source code")
	}
}

func TestDiffCompressor_TrimToMaxWords(t *testing.T) {
	// Set a small limit for testing
	os.Setenv("PR_DIFF_MAX_WORDS", "100")
	defer os.Unsetenv("PR_DIFF_MAX_WORDS")

	compressor := NewDiffCompressor()

	files := []DiffFile{
		{Path: "small.go", WordCount: 30, IsSourceCode: true},
		{Path: "medium.go", WordCount: 50, IsSourceCode: true},
		{Path: "large.go", WordCount: 80, IsSourceCode: true},
		{Path: "tiny.go", WordCount: 10, IsSourceCode: true},
	}

	result := compressor.trimToMaxWords(files)

	// Should keep tiny (10), small (30), and medium (50) = 90 words total
	// Cannot add large (80) as it would exceed 100
	if len(result) != 3 {
		t.Fatalf("Expected 3 files after trimming, got %d", len(result))
	}

	totalWords := 0
	for _, f := range result {
		totalWords += f.WordCount
	}

	if totalWords > 100 {
		t.Errorf("Total words %d exceeds max limit of 100", totalWords)
	}
}

func TestDiffCompressor_CompressDiff(t *testing.T) {
	// Set environment variables for testing - use reasonable limits
	os.Setenv("PR_DIFF_MAX_WORDS", "100")     // Increased to allow main.go
	os.Setenv("PR_DIFF_MAX_FILE_WORDS", "80") // Increased to allow individual files
	defer func() {
		os.Unsetenv("PR_DIFF_MAX_WORDS")
		os.Unsetenv("PR_DIFF_MAX_FILE_WORDS")
	}()

	compressor := NewDiffCompressor()

	// Create a test diff with various file types and enough content to trigger compression
	diff := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,10 @@
-func old() {
+func new() {
+    // This is a new function with more content
+    // to ensure we have enough words to trigger
+    // the compression logic in our test
+    fmt.Println("Hello, World!")
+    fmt.Println("This is a test")
+    fmt.Println("We need more words")
     return nil
 }
diff --git a/image.png b/image.png
Binary files differ and this is a binary file with many words that should be filtered
diff --git a/deleted.go b/deleted.go
index 111111..222222 100644
--- a/deleted.go
+++ b/deleted.go
@@ -1,5 +0,0 @@
-func removed() {
-    // This function is being deleted
-    // It should be filtered out
-    return nil
-}`

	result, err := compressor.CompressDiff(diff)
	if err != nil {
		t.Fatalf("CompressDiff failed: %v", err)
	}

	// Debug: print the result length and content
	t.Logf("Original diff length: %d, Result length: %d", len(diff), len(result))

	// The diff should be compressed (not equal to original)
	if result == diff {
		t.Error("Diff should have been compressed")
	}

	// Should keep main.go (has additions)
	if !strings.Contains(result, "main.go") {
		t.Error("Result should contain main.go")
	}
}

func TestDiffCompressor_NoCompressionNeeded(t *testing.T) {
	// Set high limits so no compression is needed
	os.Setenv("PR_DIFF_MAX_WORDS", "10000")
	os.Setenv("PR_DIFF_MAX_FILE_WORDS", "5000")
	defer func() {
		os.Unsetenv("PR_DIFF_MAX_WORDS")
		os.Unsetenv("PR_DIFF_MAX_FILE_WORDS")
	}()

	compressor := NewDiffCompressor()

	// Small diff that doesn't need compression
	diff := `diff --git a/small.go b/small.go
index abc123..def456 100644
--- a/small.go
+++ b/small.go
@@ -1,1 +1,1 @@
-func old() {}
+func new() {}`

	result, err := compressor.CompressDiff(diff)
	if err != nil {
		t.Fatalf("CompressDiff failed: %v", err)
	}

	// Should return original diff without compression
	if result != diff {
		t.Error("Small diff should not be compressed")
	}

	// Should not have compression summary
	if strings.Contains(result, "# Diff Compression Applied") {
		t.Error("Result should not contain compression summary when no compression is applied")
	}
}

func TestDiffCompressor_ProgressiveCompression(t *testing.T) {
	// Test that compression stops as soon as limits are met
	os.Setenv("PR_DIFF_MAX_WORDS", "100")     // Set a limit that will trigger compression
	os.Setenv("PR_DIFF_MAX_FILE_WORDS", "80") // Set reasonable per-file limit
	defer func() {
		os.Unsetenv("PR_DIFF_MAX_WORDS")
		os.Unsetenv("PR_DIFF_MAX_FILE_WORDS")
	}()

	compressor := NewDiffCompressor()

	// Create a diff with multiple files to test progressive compression
	// The total will exceed limits, forcing compression
	diff := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,10 +1,15 @@
 func main() {
+    // This is a change that adds some words
+    // to make sure we have enough content
     println("hello")
+    println("world")
+    println("test")
 }
diff --git a/large.pdf b/large.pdf
Binary files differ with many many words that would exceed our limit if included this is a very large pdf file with lots of content that will definitely push us over the limit and force compression to kick in
diff --git a/another.bin b/another.bin
Binary files differ this is another binary file with even more words to ensure we exceed the total word limit and trigger the compression strategies`

	result, err := compressor.CompressDiff(diff)
	if err != nil {
		t.Fatalf("CompressDiff failed: %v", err)
	}

	// Debug: print the result length
	t.Logf("Original diff length: %d, Result length: %d", len(diff), len(result))

	// The diff should be compressed
	if result == diff {
		t.Error("Diff should have been compressed - original had non-source files that should be filtered")
	}

	// Should keep main.go (it's source code and within limits)
	if !strings.Contains(result, "main.go") {
		t.Error("Should keep source files")
	}

	// Should NOT have binary files
	if strings.Contains(result, "large.pdf") || strings.Contains(result, "another.bin") {
		t.Error("Should filter out binary files")
	}
}

func TestDiffCompressor_AllFilesFiltered(t *testing.T) {
	// Test case where all files are filtered out
	os.Setenv("PR_DIFF_MAX_WORDS", "10")     // Very small limit
	os.Setenv("PR_DIFF_MAX_FILE_WORDS", "5") // Very small per-file limit
	defer func() {
		os.Unsetenv("PR_DIFF_MAX_WORDS")
		os.Unsetenv("PR_DIFF_MAX_FILE_WORDS")
	}()

	compressor := NewDiffCompressor()

	// Create a diff with only large files
	diff := `diff --git a/large.go b/large.go
index abc123..def456 100644
--- a/large.go
+++ b/large.go
@@ -1,3 +1,10 @@
-func old() {
+func new() {
+    // This file has too many words and will be filtered
+    // because it exceeds the per-file word limit
+    // we set a very small limit for testing
+    fmt.Println("Hello, World!")
+    fmt.Println("This is a test with many words")
+    fmt.Println("We need more words to exceed limit")
     return nil
 }`

	result, err := compressor.CompressDiff(diff)
	if err != nil {
		t.Fatalf("CompressDiff failed: %v", err)
	}

	// When all files are filtered out, should return empty string
	if result != "" {
		t.Errorf("Expected empty result when all files filtered, got: %q", result)
	}
}

func TestDiffCompressor_ExtractPathFromDiffLine(t *testing.T) {
	compressor := NewDiffCompressor()

	testCases := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "Standard diff line",
			line:     "diff --git a/path/to/file.go b/path/to/file.go",
			expected: "path/to/file.go",
		},
		{
			name:     "Root file",
			line:     "diff --git a/file.go b/file.go",
			expected: "file.go",
		},
		{
			name:     "Invalid format",
			line:     "not a diff line",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compressor.extractPathFromDiffLine(tc.line)
			if result != tc.expected {
				t.Errorf("extractPathFromDiffLine(%q) = %q; want %q", tc.line, result, tc.expected)
			}
		})
	}
}
