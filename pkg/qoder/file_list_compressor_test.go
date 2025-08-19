package qoder

import (
	"testing"

	"github.com/google/go-github/v73/github"
)

func TestFileListCompressor_CompressFileList(t *testing.T) {
	tests := []struct {
		name             string
		files            []*github.CommitFile
		maxTotalWords    int
		maxFileWords     int
		expectPatchCount int // Expected number of files with non-nil, non-removed patches after compression
		description      string
	}{
		{
			name: "no compression needed - under limits",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("main.go"),
					Patch:     github.Ptr("@@ -1,3 +1,4 @@\n func main() {\n+    fmt.Println(\"hello\")\n }"),
					Additions: github.Ptr(1),
				},
			},
			maxTotalWords:    1000,
			maxFileWords:     500,
			expectPatchCount: 1,
			description:      "Small files under limits should not be compressed",
		},
		{
			name: "remove non-source code files",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("main.go"),
					Patch:     github.Ptr("@@ -1,3 +1,4 @@\n func main() {\n+    fmt.Println(\"hello\")\n }"),
					Additions: github.Ptr(1),
				},
				{
					Filename:  github.Ptr("image.png"),
					Patch:     github.Ptr("Binary file changed"),
					Additions: github.Ptr(0),
				},
			},
			maxTotalWords:    10, // Total: 10 + 3 = 13 words, exceeds limit
			maxFileWords:     100,
			expectPatchCount: 1, // Only the .go file should keep its patch
			description:      "Non-source code files should have patches removed",
		},
		{
			name: "remove large files",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("small.go"),
					Patch:     github.Ptr("small patch"),
					Additions: github.Ptr(1),
				},
				{
					Filename:  github.Ptr("large.go"),
					Patch:     github.Ptr("this is a very large patch with many many many many many many many many words that exceeds the file limit"),
					Additions: github.Ptr(10),
				},
			},
			maxTotalWords:    20, // Total words (2+21=23) exceed this, triggering compression
			maxFileWords:     10, // Large file also exceeds this
			expectPatchCount: 1,  // Only small.go should keep its patch
			description:      "Files with patches exceeding maxFileWords should be removed",
		},
		{
			name: "remove deletion-only files",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("new.go"),
					Patch:     github.Ptr("@@ -0,0 +1,3 @@\n+func new() {\n+    return\n+}"),
					Additions: github.Ptr(3),
				},
				{
					Filename:  github.Ptr("deleted.go"),
					Patch:     github.Ptr("@@ -1,3 +0,0 @@\n-func old() {\n-    return\n-}"),
					Additions: github.Ptr(0),
					Deletions: github.Ptr(3),
				},
			},
			maxTotalWords:    10,
			maxFileWords:     100,
			expectPatchCount: 1, // Only new.go should keep its patch
			description:      "Deletion-only files should have patches removed",
		},
		{
			name: "keep only most important files",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("important.go"),
					Patch:     github.Ptr("@@ -1,2 +1,3 @@\n package main\n+func important() {}"),
					Additions: github.Ptr(5),
				},
				{
					Filename:  github.Ptr("config.json"),
					Patch:     github.Ptr("@@ -1,1 +1,2 @@\n {\n+  \"new\": true"),
					Additions: github.Ptr(2),
				},
				{
					Filename:  github.Ptr("other.go"),
					Patch:     github.Ptr("@@ -1,1 +1,2 @@\n package other\n+func other() {}"),
					Additions: github.Ptr(1),
				},
			},
			maxTotalWords:    15, // Total: 8+6+7=21 words, exceeds limit, should keep most important
			maxFileWords:     100,
			expectPatchCount: 1, // Should keep only the most important file after compression
			description:      "Should prioritize source code files and files with more additions",
		},
		{
			name: "no compression when under limits",
			files: []*github.CommitFile{
				{
					Filename:  github.Ptr("test.go"),
					Patch:     github.Ptr("small"),
					Additions: github.Ptr(1),
				},
			},
			maxTotalWords:    100000, // Very high limit
			maxFileWords:     50000,  // Very high limit
			expectPatchCount: 1,      // Should return the file unchanged
			description:      "Files under limits should not be compressed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressor := &FileListCompressor{
				maxTotalWords: tt.maxTotalWords,
				maxFileWords:  tt.maxFileWords,
			}

			result := compressor.CompressFileList(tt.files)

			// Count files with actual patch content (not compression messages)
			actualPatchCount := 0
			for _, file := range result {
				if file.Patch != nil && !isCompressionMessage(*file.Patch) {
					actualPatchCount++
				}
			}

			if actualPatchCount != tt.expectPatchCount {
				t.Errorf("%s: expected %d files with patches, got %d",
					tt.description, tt.expectPatchCount, actualPatchCount)

				// Debug info
				for i, file := range result {
					patchStatus := "nil"
					if file.Patch != nil {
						if isCompressionMessage(*file.Patch) {
							patchStatus = "removed: " + *file.Patch
						} else {
							patchStatus = "kept: " + *file.Patch
						}
					}
					t.Logf("File %d: %s - patch %s", i, *file.Filename, patchStatus)
				}
			}

			// Verify total word count is within limits (considering only kept patches)
			totalWords := 0
			for _, file := range result {
				if file.Patch != nil && !isCompressionMessage(*file.Patch) {
					totalWords += compressor.countWords(*file.Patch)
				}
			}

			if totalWords > tt.maxTotalWords {
				t.Errorf("%s: total words %d exceeds limit %d",
					tt.description, totalWords, tt.maxTotalWords)
			}
		})
	}
}

func TestFileListCompressor_isSourceCodeFile(t *testing.T) {
	compressor := &FileListCompressor{}

	tests := []struct {
		filename string
		expected bool
	}{
		{"main.go", true},
		{"script.py", true},
		{"app.js", true},
		{"style.css", true},
		{"README.md", true},
		{"Dockerfile", true},
		{"image.png", false},
		{"binary.exe", false},
		{"data.bin", false},
		{"archive.zip", false},
		{"video.mp4", false},
		{"package-lock.json", true}, // Should be considered source code
		{"go.sum", true},            // Should be considered source code
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := compressor.isSourceCodeFile(tt.filename)
			if result != tt.expected {
				t.Errorf("isSourceCodeFile(%s) = %v, expected %v",
					tt.filename, result, tt.expected)
			}
		})
	}
}

func TestFileListCompressor_isDeletionOnlyFile(t *testing.T) {
	compressor := &FileListCompressor{}

	tests := []struct {
		name     string
		file     *github.CommitFile
		expected bool
	}{
		{
			name: "deletion only with statistics",
			file: &github.CommitFile{
				Filename:  github.Ptr("deleted.go"),
				Changes:   github.Ptr(3),
				Deletions: github.Ptr(3),
				Additions: github.Ptr(0),
				Patch:     github.Ptr("@@ -1,3 +0,0 @@\n-func old() {\n-    return\n-}"),
			},
			expected: true,
		},
		{
			name: "addition only with statistics",
			file: &github.CommitFile{
				Filename:  github.Ptr("new.go"),
				Changes:   github.Ptr(3),
				Deletions: github.Ptr(0),
				Additions: github.Ptr(3),
				Patch:     github.Ptr("@@ -0,0 +1,3 @@\n+func new() {\n+    return\n+}"),
			},
			expected: false,
		},
		{
			name: "mixed changes with statistics",
			file: &github.CommitFile{
				Filename:  github.Ptr("modified.go"),
				Changes:   github.Ptr(2),
				Deletions: github.Ptr(1),
				Additions: github.Ptr(1),
				Patch:     github.Ptr("@@ -1,3 +1,3 @@\n-func old() {\n+func new() {\n     return\n }"),
			},
			expected: false,
		},
		{
			name: "deletion only fallback to patch analysis",
			file: &github.CommitFile{
				Filename: github.Ptr("deleted.go"),
				Patch:    github.Ptr("@@ -1,3 +0,0 @@\n-func old() {\n-    return\n-}"),
			},
			expected: true,
		},
		{
			name: "context only fallback to patch analysis",
			file: &github.CommitFile{
				Filename: github.Ptr("same.go"),
				Patch:    github.Ptr("@@ -1,3 +1,3 @@\n func same() {\n     return\n }"),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compressor.isDeletionOnlyFile(tt.file)
			if result != tt.expected {
				t.Errorf("isDeletionOnlyFile() = %v, expected %v for file: %s",
					result, tt.expected, tt.file.GetFilename())
			}
		})
	}
}

func TestFileListCompressor_countWords(t *testing.T) {
	compressor := &FileListCompressor{}

	tests := []struct {
		text     string
		expected int
	}{
		{"hello world", 2},
		{"one", 1},
		{"", 0},
		{"  multiple   spaces   between   words  ", 4},
		{"line1\nline2\nline3", 3},
		{"@@ -1,3 +1,4 @@\n func main() {\n+    fmt.Println(\"hello\")\n }", 10},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := compressor.countWords(tt.text)
			if result != tt.expected {
				t.Errorf("countWords(%q) = %d, expected %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestFileListCompressor_calculateTotalWords(t *testing.T) {
	compressor := &FileListCompressor{}

	files := []*github.CommitFile{
		{
			Filename: github.Ptr("file1.go"),
			Patch:    github.Ptr("hello world"),
		},
		{
			Filename: github.Ptr("file2.go"),
			Patch:    github.Ptr("one two three"),
		},
		{
			Filename: github.Ptr("file3.go"),
			Patch:    nil, // Should be ignored
		},
		{
			Filename: github.Ptr("file4.go"),
			Patch:    github.Ptr("[Patch removed: non-source code file]"),
		},
	}

	totalWords := compressor.calculateTotalWords(files)
	expected := 2 + 3 + 5 // "hello world" + "one two three" + "[Patch removed: non-source code file]"
	if totalWords != expected {
		t.Errorf("calculateTotalWords() = %d, expected %d", totalWords, expected)
	}
}

func TestFileListCompressor_NewFileListCompressor(t *testing.T) {
	// Test default values
	compressor := NewFileListCompressor()
	if compressor.maxTotalWords != 50000 {
		t.Errorf("Expected default maxTotalWords 50000, got %d", compressor.maxTotalWords)
	}
	if compressor.maxFileWords != 5000 {
		t.Errorf("Expected default maxFileWords 5000, got %d", compressor.maxFileWords)
	}
}

// Helper function to check if a patch is a compression message
func isCompressionMessage(patch string) bool {
	compressionMessages := []string{
		"[Patch removed: non-source code file]",
		"[Patch removed: file too large]",
		"[Patch removed: deletion-only file]",
		"[Patch removed: reached word limit]",
	}

	for _, msg := range compressionMessages {
		if patch == msg {
			return true
		}
	}
	return false
}

// Test the compression with realistic PR data
func TestFileListCompressor_RealisticPRData(t *testing.T) {
	compressor := &FileListCompressor{
		maxTotalWords: 100,
		maxFileWords:  50,
	}

	// Simulate a realistic PR with various file types
	files := []*github.CommitFile{
		{
			Filename:  github.Ptr("main.go"),
			Patch:     github.Ptr("@@ -1,10 +1,15 @@\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n+\tfmt.Println(\"Hello, World!\")\n+\tfmt.Println(\"This is a new line\")\n\tprintln(\"Original line\")\n}"),
			Additions: github.Ptr(2),
			Deletions: github.Ptr(0),
		},
		{
			Filename:  github.Ptr("package-lock.json"),
			Patch:     github.Ptr("very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very very long package lock content that exceeds the file word limit"),
			Additions: github.Ptr(100),
			Deletions: github.Ptr(0),
		},
		{
			Filename:  github.Ptr("deleted_file.go"),
			Patch:     github.Ptr("@@ -1,5 +0,0 @@\n-package main\n-\n-func deleted() {\n-\treturn\n-}"),
			Additions: github.Ptr(0),
			Deletions: github.Ptr(5),
		},
		{
			Filename:  github.Ptr("image.png"),
			Patch:     github.Ptr("Binary files a/image.png and b/image.png differ"),
			Additions: github.Ptr(0),
			Deletions: github.Ptr(0),
		},
		{
			Filename:  github.Ptr("utils.go"),
			Patch:     github.Ptr("@@ -1,3 +1,5 @@\npackage utils\n\n+func helper() {\n+}\n\nfunc existing() {}"),
			Additions: github.Ptr(2),
			Deletions: github.Ptr(0),
		},
	}

	result := compressor.CompressFileList(files)

	// Count actual patches kept
	keptPatches := 0
	removedPatches := 0

	for _, file := range result {
		if file.Patch != nil {
			if isCompressionMessage(*file.Patch) {
				removedPatches++
			} else {
				keptPatches++
			}
		}
	}

	t.Logf("Kept patches: %d, Removed patches: %d", keptPatches, removedPatches)

	// Should prioritize source code files and remove non-source/large/deletion-only files
	if keptPatches == 0 {
		t.Error("Should keep at least some source code files")
	}

	if removedPatches == 0 {
		t.Error("Should remove some files due to compression limits")
	}

	// Verify total words is within limit
	totalWords := compressor.calculateTotalWords(result)
	actualKeptWords := 0
	for _, file := range result {
		if file.Patch != nil && !isCompressionMessage(*file.Patch) {
			actualKeptWords += compressor.countWords(*file.Patch)
		}
	}

	t.Logf("Total words in result: %d, Actual kept words: %d, Limit: %d",
		totalWords, actualKeptWords, compressor.maxTotalWords)

	if actualKeptWords > compressor.maxTotalWords {
		t.Errorf("Kept words %d exceeds limit %d", actualKeptWords, compressor.maxTotalWords)
	}
}
