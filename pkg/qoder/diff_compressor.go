package qoder

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-github/v73/github"
)

// DiffFile represents a file in the diff
type DiffFile struct {
	Path             string
	Content          string
	WordCount        int
	IsSourceCode     bool
	HasOnlyDeletions bool
}

// DiffCompressor handles compression of PR diffs
type DiffCompressor struct {
	maxWords     int
	maxFileWords int
}

// NewDiffCompressor creates a new diff compressor with environment variable configuration
func NewDiffCompressor() *DiffCompressor {
	maxWords := 50000    // default
	maxFileWords := 5000 // default

	if envVal := os.Getenv("PR_DIFF_MAX_WORDS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			maxWords = val
		}
	}

	if envVal := os.Getenv("PR_DIFF_MAX_FILE_WORDS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			maxFileWords = val
		}
	}

	return &DiffCompressor{
		maxWords:     maxWords,
		maxFileWords: maxFileWords,
	}
}

// CompressDiff applies compression strategies to reduce diff size
//
// The compression is applied progressively in the following order:
// 1. Filter out non-source code files (images, binaries, lock files, etc.)
//   - Removes files like .png, .jpg, .pdf, .exe, package-lock.json, go.sum
//   - Keeps source code files (.go, .py, .js, etc.) and important configs
//
// 2. Filter out files exceeding maxFileWords limit
//   - Removes individual files that are larger than PR_DIFF_MAX_FILE_WORDS
//   - Default limit is 5000 words per file
//
// 3. Filter out deletion-only files
//   - Removes files that only contain deletions (no additions)
//   - These are less relevant for understanding new functionality
//
// 4. Trim by size if still exceeding total word limit
//   - Sort files by word count (largest first)
//   - Keep as many small files as possible within PR_DIFF_MAX_WORDS limit
//   - Default total limit is 50000 words
//
// The compression stops as soon as the content fits within both limits:
// - Total words <= PR_DIFF_MAX_WORDS
// - Each file <= PR_DIFF_MAX_FILE_WORDS
//
// If no compression is needed (content already within limits), returns original diff.
// Returns compressed diff with summary header indicating which strategy was applied.
func (c *DiffCompressor) CompressDiff(rawDiff string) (string, error) {
	files := c.parseDiffIntoFiles(rawDiff)

	// Check if compression is needed
	totalWords := c.getTotalWords(files)
	if totalWords <= c.maxWords && !c.hasOversizedFiles(files) {
		// No compression needed
		return rawDiff, nil
	}

	// Apply compression strategies in order, stop when limits are met

	// Strategy 1: Remove non-source code files
	files = c.filterSourceCodeFiles(files)
	if c.isWithinLimits(files) {
		return c.reconstructDiff(files), nil
	}

	// Strategy 2: Remove files exceeding maxFileWords
	files = c.filterLargeFiles(files)
	if c.isWithinLimits(files) {
		return c.reconstructDiff(files), nil
	}

	// Strategy 3: Remove files with only deletions
	files = c.filterDeletionOnlyFiles(files)
	if c.isWithinLimits(files) {
		return c.reconstructDiff(files), nil
	}

	// Strategy 4: If still exceeding limit, remove largest files
	files = c.trimToMaxWords(files)
	return c.reconstructDiff(files), nil
}

// parseDiffIntoFiles splits the raw diff into individual file diffs
func (c *DiffCompressor) parseDiffIntoFiles(rawDiff string) []DiffFile {
	var files []DiffFile
	lines := strings.Split(rawDiff, "\n")

	var currentFile *DiffFile
	var currentContent []string

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git") {
			// Save previous file if exists
			if currentFile != nil {
				currentFile.Content = strings.Join(currentContent, "\n")
				currentFile.WordCount = c.countWords(currentFile.Content)
				currentFile.IsSourceCode = c.isSourceCodeFile(currentFile.Path)
				currentFile.HasOnlyDeletions = c.hasOnlyDeletions(currentFile.Content)
				files = append(files, *currentFile)
			}

			// Start new file
			path := c.extractPathFromDiffLine(line)
			currentFile = &DiffFile{Path: path}
			currentContent = []string{line}
		} else if currentFile != nil {
			currentContent = append(currentContent, line)
		}
	}

	// Don't forget the last file
	if currentFile != nil {
		currentFile.Content = strings.Join(currentContent, "\n")
		currentFile.WordCount = c.countWords(currentFile.Content)
		currentFile.IsSourceCode = c.isSourceCodeFile(currentFile.Path)
		currentFile.HasOnlyDeletions = c.hasOnlyDeletions(currentFile.Content)
		files = append(files, *currentFile)
	}

	return files
}

// extractPathFromDiffLine extracts file path from diff --git line
func (c *DiffCompressor) extractPathFromDiffLine(line string) string {
	// Check if it's actually a diff line
	if !strings.HasPrefix(line, "diff --git") {
		return ""
	}

	// Format: diff --git a/path/to/file b/path/to/file
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		path := parts[2]
		if strings.HasPrefix(path, "a/") {
			return path[2:]
		}
		return path
	}
	return ""
}

// countWords counts words in the content
func (c *DiffCompressor) countWords(content string) int {
	return len(strings.Fields(content))
}

// isSourceCodeFile checks if a file is source code based on extension
func (c *DiffCompressor) isSourceCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)

	// First check if it's a lock file or similar (should be excluded)
	excludedFiles := map[string]bool{
		"package-lock.json": true,
		"yarn.lock":         true,
		"pnpm-lock.yaml":    true,
		"go.sum":            true,
		"Gemfile.lock":      true,
		"Cargo.lock":        true,
		"composer.lock":     true,
		"poetry.lock":       true,
	}

	if excludedFiles[base] {
		return false
	}

	// Source code extensions whitelist
	sourceExts := map[string]bool{
		".go":     true,
		".py":     true,
		".js":     true,
		".jsx":    true,
		".ts":     true,
		".tsx":    true,
		".java":   true,
		".c":      true,
		".cpp":    true,
		".cc":     true,
		".h":      true,
		".hpp":    true,
		".cs":     true,
		".rb":     true,
		".php":    true,
		".swift":  true,
		".kt":     true,
		".rs":     true,
		".scala":  true,
		".sh":     true,
		".bash":   true,
		".zsh":    true,
		".fish":   true,
		".ps1":    true,
		".r":      true,
		".m":      true,
		".mm":     true,
		".sql":    true,
		".html":   true,
		".css":    true,
		".scss":   true,
		".sass":   true,
		".less":   true,
		".vue":    true,
		".svelte": true,
	}

	// Configuration files that might be important (but not lock files)
	configFiles := map[string]bool{
		".yml":        true,
		".yaml":       true,
		".json":       true,
		".xml":        true,
		".toml":       true,
		".ini":        true,
		".cfg":        true,
		".conf":       true,
		".properties": true,
		".env":        true,
		"Dockerfile":  true,
		"Makefile":    true,
		".gitignore":  true,
	}

	// Check if it's a source file
	if sourceExts[ext] {
		return true
	}

	// Check if it's an important config file (but not a lock file)
	if configFiles[ext] || configFiles[base] {
		return true
	}

	// Check for files without extension that might be scripts
	if ext == "" {
		// Common script files without extensions
		if base == "Makefile" || base == "Dockerfile" || base == "Jenkinsfile" || base == "Rakefile" {
			return true
		}
	}

	// Check for README files
	if strings.Contains(strings.ToLower(path), "readme") {
		return true
	}

	return false
}

// hasOnlyDeletions checks if a file diff contains only deletions
func (c *DiffCompressor) hasOnlyDeletions(content string) bool {
	lines := strings.Split(content, "\n")
	hasAdditions := false
	hasDeletions := false

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// Check diff lines (skip headers)
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			hasAdditions = true
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			hasDeletions = true
		}

		// Also check for enhanced diff format with line numbers
		if len(line) > 4 {
			// Format could be "123 +content" or "   -content"
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 0 {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					secondPart := parts[1]
					if strings.HasPrefix(secondPart, "+") {
						hasAdditions = true
					}
				} else if len(parts) == 1 && strings.HasPrefix(parts[0], "-") {
					hasDeletions = true
				}
			}
		}
	}

	return hasDeletions && !hasAdditions
}

// filterSourceCodeFiles removes non-source code files
func (c *DiffCompressor) filterSourceCodeFiles(files []DiffFile) []DiffFile {
	var filtered []DiffFile
	for _, file := range files {
		if file.IsSourceCode {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// filterLargeFiles removes files exceeding maxFileWords
func (c *DiffCompressor) filterLargeFiles(files []DiffFile) []DiffFile {
	var filtered []DiffFile
	for _, file := range files {
		if file.WordCount <= c.maxFileWords {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// filterDeletionOnlyFiles removes files with only deletions
func (c *DiffCompressor) filterDeletionOnlyFiles(files []DiffFile) []DiffFile {
	var filtered []DiffFile
	for _, file := range files {
		if !file.HasOnlyDeletions {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// trimToMaxWords removes largest files until total is under maxWords
func (c *DiffCompressor) trimToMaxWords(files []DiffFile) []DiffFile {
	totalWords := 0
	for _, file := range files {
		totalWords += file.WordCount
	}

	if totalWords <= c.maxWords {
		return files
	}

	// Sort by word count (largest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].WordCount > files[j].WordCount
	})

	var result []DiffFile
	currentTotal := 0

	// Add files from smallest to largest until we hit the limit
	for i := len(files) - 1; i >= 0; i-- {
		if currentTotal+files[i].WordCount <= c.maxWords {
			result = append(result, files[i])
			currentTotal += files[i].WordCount
		}
	}

	return result
}

// getTotalWords calculates total words across all files
func (c *DiffCompressor) getTotalWords(files []DiffFile) int {
	total := 0
	for _, file := range files {
		total += file.WordCount
	}
	return total
}

// hasOversizedFiles checks if any file exceeds maxFileWords
func (c *DiffCompressor) hasOversizedFiles(files []DiffFile) bool {
	for _, file := range files {
		if file.WordCount > c.maxFileWords {
			return true
		}
	}
	return false
}

// isWithinLimits checks if files are within both total and per-file limits
func (c *DiffCompressor) isWithinLimits(files []DiffFile) bool {
	totalWords := c.getTotalWords(files)
	if totalWords > c.maxWords {
		return false
	}

	for _, file := range files {
		if file.WordCount > c.maxFileWords {
			return false
		}
	}

	return true
}

// reconstructDiff rebuilds the diff string from filtered files
func (c *DiffCompressor) reconstructDiff(files []DiffFile) string {
	if len(files) == 0 {
		return ""
	}

	var parts []string
	for _, file := range files {
		parts = append(parts, file.Content)
	}

	return strings.Join(parts, "\n")
}

// FileListCompressor handles compression of PR file lists by removing patch content when needed
type FileListCompressor struct {
	maxTotalWords int
	maxFileWords  int
}

// NewFileListCompressor creates a new file list compressor with environment variable configuration
func NewFileListCompressor() *FileListCompressor {
	maxTotalWords := 50000 // default total words limit
	maxFileWords := 5000   // default per-file words limit

	if envVal := os.Getenv("PR_DIFF_MAX_WORDS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			maxTotalWords = val
		}
	}

	if envVal := os.Getenv("PR_DIFF_MAX_FILE_WORDS"); envVal != "" {
		if val, err := strconv.Atoi(envVal); err == nil && val > 0 {
			maxFileWords = val
		}
	}

	return &FileListCompressor{
		maxTotalWords: maxTotalWords,
		maxFileWords:  maxFileWords,
	}
}

// CompressFileList applies compression strategies to file list by removing patch content when needed
func (c *FileListCompressor) CompressFileList(files []*github.CommitFile) []*github.CommitFile {
	// Calculate total patch size
	totalWords := 0
	for _, file := range files {
		if file.Patch != nil {
			totalWords += c.countWords(*file.Patch)
		}
	}

	// If within limits, return as-is
	if totalWords <= c.maxTotalWords {
		return files
	}

	// Create a deep copy to avoid modifying original
	compressedFiles := make([]*github.CommitFile, len(files))
	for i, file := range files {
		// Create a deep copy of the file
		fileCopy := &github.CommitFile{
			SHA:              file.SHA,
			Filename:         file.Filename,
			Additions:        file.Additions,
			Deletions:        file.Deletions,
			Changes:          file.Changes,
			Status:           file.Status,
			RawURL:           file.RawURL,
			BlobURL:          file.BlobURL,
			ContentsURL:      file.ContentsURL,
			PreviousFilename: file.PreviousFilename,
		}
		// Deep copy the patch string
		if file.Patch != nil {
			patchCopy := *file.Patch
			fileCopy.Patch = &patchCopy
		}
		compressedFiles[i] = fileCopy
	}

	// Apply compression strategies progressively until within limits

	// Strategy 1: Remove patch from non-source code files
	compressedFiles = c.removePatchFromNonSourceFiles(compressedFiles)
	if c.calculateTotalWords(compressedFiles) <= c.maxTotalWords {
		return compressedFiles
	}

	// Strategy 2: Remove patch from large files
	compressedFiles = c.removePatchFromLargeFiles(compressedFiles)
	if c.calculateTotalWords(compressedFiles) <= c.maxTotalWords {
		return compressedFiles
	}

	// Strategy 3: Remove patch from deletion-only files
	compressedFiles = c.removePatchFromDeletionOnlyFiles(compressedFiles)
	if c.calculateTotalWords(compressedFiles) <= c.maxTotalWords {
		return compressedFiles
	}

	// Strategy 4: Keep only the most important files with patches
	compressedFiles = c.keepOnlyImportantFilePatches(compressedFiles)

	return compressedFiles
}

// countWords counts words in a string
func (c *FileListCompressor) countWords(text string) int {
	return len(strings.Fields(text))
}

// calculateTotalWords calculates total words in all patches
func (c *FileListCompressor) calculateTotalWords(files []*github.CommitFile) int {
	total := 0
	for _, file := range files {
		if file.Patch != nil {
			total += c.countWords(*file.Patch)
		}
	}
	return total
}

// removePatchFromNonSourceFiles removes patch content from non-source code files
func (c *FileListCompressor) removePatchFromNonSourceFiles(files []*github.CommitFile) []*github.CommitFile {
	for _, file := range files {
		if file.Filename != nil && !c.isSourceCodeFile(*file.Filename) {
			// Keep file metadata but remove patch content
			file.Patch = github.Ptr("[Patch removed: non-source code file]")
		}
	}
	return files
}

// removePatchFromLargeFiles removes patch content from files with large patches
func (c *FileListCompressor) removePatchFromLargeFiles(files []*github.CommitFile) []*github.CommitFile {
	for _, file := range files {
		if file.Patch != nil && c.countWords(*file.Patch) > c.maxFileWords {
			// Keep file metadata but remove patch content
			file.Patch = github.Ptr("[Patch removed: file too large]")
		}
	}
	return files
}

// removePatchFromDeletionOnlyFiles removes patch content from files with only deletions
func (c *FileListCompressor) removePatchFromDeletionOnlyFiles(files []*github.CommitFile) []*github.CommitFile {
	for _, file := range files {
		if file.Patch != nil && c.isDeletionOnlyFile(file) {
			// Keep file metadata but remove patch content
			file.Patch = github.Ptr("[Patch removed: deletion-only file]")
		}
	}
	return files
}

// keepOnlyImportantFilePatches keeps patches only for the most important files
func (c *FileListCompressor) keepOnlyImportantFilePatches(files []*github.CommitFile) []*github.CommitFile {
	// Sort files by importance (source code files first, then by additions count)
	type fileWithWords struct {
		file  *github.CommitFile
		words int
		score int // higher score = more important
	}

	var fileStats []fileWithWords
	for _, file := range files {
		if file.Patch == nil {
			continue
		}

		words := c.countWords(*file.Patch)
		score := 0

		// Source code files get higher score
		if file.Filename != nil && c.isSourceCodeFile(*file.Filename) {
			score += 1000
		}

		// Files with more additions get higher score
		if file.Additions != nil {
			score += *file.Additions
		}

		fileStats = append(fileStats, fileWithWords{file, words, score})
	}

	// Sort by score (descending) then by words (ascending - prefer smaller files)
	sort.Slice(fileStats, func(i, j int) bool {
		if fileStats[i].score == fileStats[j].score {
			return fileStats[i].words < fileStats[j].words
		}
		return fileStats[i].score > fileStats[j].score
	})

	// Keep patches for important files within word limit
	currentWords := 0
	for _, stat := range fileStats {
		if currentWords+stat.words <= c.maxTotalWords {
			currentWords += stat.words
		} else {
			// Remove patch from this file
			stat.file.Patch = github.Ptr("[Patch removed: reached word limit]")
		}
	}

	return files
}

// isSourceCodeFile checks if a file is source code based on extension
func (c *FileListCompressor) isSourceCodeFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	sourceExts := []string{
		".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".c", ".cpp", ".h", ".hpp",
		".cs", ".php", ".rb", ".rs", ".kt", ".swift", ".scala", ".clj", ".ml", ".hs",
		".sh", ".bash", ".zsh", ".ps1", ".sql", ".r", ".m", ".pl", ".lua", ".dart",
		".vue", ".svelte", ".elm", ".ex", ".exs", ".cr", ".nim", ".zig", ".v",
		".html", ".css", ".scss", ".sass", ".less", ".xml", ".yaml", ".yml", ".json",
		".toml", ".ini", ".cfg", ".conf", ".properties", ".env", ".gitignore",
		".dockerfile", ".makefile", ".md", ".rst", ".txt",
	}

	for _, sourceExt := range sourceExts {
		if ext == sourceExt {
			return true
		}
	}

	// Check special filenames without extensions
	baseName := strings.ToLower(filepath.Base(filename))
	specialFiles := []string{
		"makefile", "dockerfile", "rakefile", "gemfile", "podfile",
		"readme", "license", "changelog", "contributing",
		"go.sum", "go.mod", "package-lock.json", "yarn.lock",
	}

	for _, special := range specialFiles {
		if baseName == special {
			return true
		}
	}

	return false
}

// isDeletionOnlyFile checks if a file contains only deletions
func (c *FileListCompressor) isDeletionOnlyFile(file *github.CommitFile) bool {
	// If we have the statistics, use them for a more accurate check
	if file.Changes != nil && file.Deletions != nil {
		// If all changes are deletions, it's a deletion-only file
		return *file.Changes == *file.Deletions && *file.Changes > 0
	}

	// Fallback to patch analysis if statistics are not available
	if file.Patch == nil {
		return false
	}

	lines := strings.Split(*file.Patch, "\n")
	hasAdditions := false

	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			hasAdditions = true
			break
		}
	}

	return !hasAdditions
}
