package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrPathTraversal = fmt.Errorf("path traversal detected")
	ErrAbsolutePath  = fmt.Errorf("absolute paths are not allowed")
	ErrReservedName  = fmt.Errorf("reserved filename not allowed")

	windowsReservedNames = map[string]bool{
		"con": true, "prn": true, "aux": true, "nul": true,
		"com1": true, "com2": true, "com3": true, "com4": true,
		"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
		"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
	}
)

func ValidateSavePath(path string) error {
	if filepath.IsAbs(path) {
		return ErrAbsolutePath
	}

	cleaned := filepath.Clean(path)

	if strings.HasPrefix(cleaned, "..") || strings.Contains(path, "..") {
		return ErrPathTraversal
	}

	base := filepath.Base(cleaned)
	nameWithoutExt := strings.TrimSuffix(strings.ToLower(base), filepath.Ext(base))

	if windowsReservedNames[nameWithoutExt] {
		return ErrReservedName
	}

	if strings.HasPrefix(base, "-") {
		return fmt.Errorf("filename cannot start with hyphen")
	}

	return nil
}

func SanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-",
		"*", "", "?", "", "\"", "",
		"<", "", ">", "", "|", "", "\x00", "",
	)
	sanitized := replacer.Replace(name)
	sanitized = strings.TrimLeft(sanitized, ".-")
	sanitized = strings.TrimRight(sanitized, ". ")

	nameWithoutExt := strings.TrimSuffix(strings.ToLower(sanitized), filepath.Ext(sanitized))
	if windowsReservedNames[nameWithoutExt] {
		sanitized = sanitized + "_"
	}

	if sanitized == "" {
		sanitized = "file"
	}

	return sanitized
}
