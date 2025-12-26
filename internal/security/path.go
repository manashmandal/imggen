package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

var (
	// ErrPathTraversal is returned when path contains traversal sequences
	ErrPathTraversal = fmt.Errorf("path traversal detected")

	// ErrAbsolutePath is returned when an absolute path is provided where relative is expected
	ErrAbsolutePath = fmt.Errorf("absolute paths are not allowed")

	// ErrReservedName is returned when filename is a Windows reserved name
	ErrReservedName = fmt.Errorf("reserved filename not allowed")

	// windowsReservedNames contains names that are reserved on Windows
	windowsReservedNames = map[string]bool{
		"con": true, "prn": true, "aux": true, "nul": true,
		"com1": true, "com2": true, "com3": true, "com4": true,
		"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
		"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
	}
)

// ValidateSavePath validates a path intended for saving files
// It checks for path traversal and other unsafe patterns
func ValidateSavePath(path string) error {
	// Check for absolute paths
	if filepath.IsAbs(path) {
		return ErrAbsolutePath
	}

	// Clean the path and check for traversal
	cleaned := filepath.Clean(path)

	// Check if cleaned path tries to escape current directory
	if strings.HasPrefix(cleaned, "..") {
		return ErrPathTraversal
	}

	// Check for path traversal sequences
	if strings.Contains(path, "..") {
		return ErrPathTraversal
	}

	// Get the base filename for reserved name check
	base := filepath.Base(cleaned)
	baseLower := strings.ToLower(base)

	// Remove extension for reserved name check
	ext := filepath.Ext(baseLower)
	nameWithoutExt := strings.TrimSuffix(baseLower, ext)

	// Check for Windows reserved names
	if windowsReservedNames[nameWithoutExt] {
		return ErrReservedName
	}

	// Check for names starting with hyphen (could be interpreted as flags)
	if strings.HasPrefix(base, "-") {
		return fmt.Errorf("filename cannot start with hyphen")
	}

	return nil
}

// SanitizeFilename sanitizes a filename for safe filesystem use
func SanitizeFilename(name string) string {
	// Remove or replace dangerous characters
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
		"\x00", "",
	)
	sanitized := replacer.Replace(name)

	// Remove leading dots and hyphens
	sanitized = strings.TrimLeft(sanitized, ".-")

	// Remove trailing dots and spaces (Windows issue)
	sanitized = strings.TrimRight(sanitized, ". ")

	// Check for Windows reserved names and append underscore if needed
	baseLower := strings.ToLower(sanitized)
	ext := filepath.Ext(baseLower)
	nameWithoutExt := strings.TrimSuffix(baseLower, ext)

	if windowsReservedNames[nameWithoutExt] {
		sanitized = sanitized + "_"
	}

	// If empty after sanitization, use default
	if sanitized == "" {
		sanitized = "file"
	}

	return sanitized
}
