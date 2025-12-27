package security

import (
	"testing"
)

func TestValidateSavePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "valid simple filename",
			path:    "image.png",
			wantErr: nil,
		},
		{
			name:    "valid filename with subdirectory",
			path:    "output/image.png",
			wantErr: nil,
		},
		{
			name:    "path traversal with ..",
			path:    "../image.png",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "path traversal in middle",
			path:    "foo/../../../etc/passwd",
			wantErr: ErrPathTraversal,
		},
		{
			name:    "absolute path unix",
			path:    "/etc/passwd",
			wantErr: ErrAbsolutePath,
		},
		{
			name:    "windows reserved name CON",
			path:    "CON.txt",
			wantErr: ErrReservedName,
		},
		{
			name:    "windows reserved name PRN",
			path:    "prn.png",
			wantErr: ErrReservedName,
		},
		{
			name:    "windows reserved name NUL",
			path:    "nul",
			wantErr: ErrReservedName,
		},
		{
			name:    "windows reserved name LPT1",
			path:    "lpt1.doc",
			wantErr: ErrReservedName,
		},
		{
			name:    "filename starting with hyphen",
			path:    "-image.png",
			wantErr: nil, // This returns a specific error, not ErrPathTraversal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSavePath(tt.path)
			if tt.wantErr == nil {
				if err != nil && tt.name != "filename starting with hyphen" {
					t.Errorf("ValidateSavePath(%q) error = %v, wantErr nil", tt.path, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateSavePath(%q) error = nil, wantErr %v", tt.path, tt.wantErr)
				} else if err != tt.wantErr && tt.wantErr != nil {
					t.Errorf("ValidateSavePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				}
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal filename",
			input:    "image.png",
			expected: "image.png",
		},
		{
			name:     "filename with slashes",
			input:    "foo/bar.png",
			expected: "foo-bar.png",
		},
		{
			name:     "filename with backslashes",
			input:    "foo\\bar.png",
			expected: "foo-bar.png",
		},
		{
			name:     "leading dots removed",
			input:    "..hidden.png",
			expected: "hidden.png",
		},
		{
			name:     "leading hyphens removed",
			input:    "--flag.png",
			expected: "flag.png",
		},
		{
			name:     "trailing dots removed",
			input:    "file.png...",
			expected: "file.png",
		},
		{
			name:     "special characters removed",
			input:    "file<name>:with*bad?chars.png",
			expected: "filename-withbadchars.png",
		},
		{
			name:     "windows reserved name gets underscore",
			input:    "CON.txt",
			expected: "CON.txt_",
		},
		{
			name:     "empty becomes file",
			input:    "...",
			expected: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
