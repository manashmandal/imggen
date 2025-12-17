package display

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestKittyEncoder_Encode_Empty(t *testing.T) {
	var buf bytes.Buffer
	enc := NewKittyEncoder(&buf)

	err := enc.Encode([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestKittyEncoder_Encode_SmallImage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewKittyEncoder(&buf)

	data := []byte("small test data")
	err := enc.Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.HasPrefix(output, "\x1b_G") {
		t.Error("output should start with escape sequence")
	}
	if !strings.HasSuffix(output, "\x1b\\") {
		t.Error("output should end with escape terminator")
	}
	if !strings.Contains(output, "a=T") {
		t.Error("output should contain action=transmit")
	}
	if !strings.Contains(output, "f=100") {
		t.Error("output should contain format=100 (PNG)")
	}
	if !strings.Contains(output, "q=2") {
		t.Error("output should contain quiet mode")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	if !strings.Contains(output, encoded) {
		t.Error("output should contain base64 encoded data")
	}
}

func TestKittyEncoder_Encode_LargeImage(t *testing.T) {
	var buf bytes.Buffer
	enc := NewKittyEncoder(&buf)

	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(i % 256)
	}

	err := enc.Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	escCount := strings.Count(output, "\x1b_G")
	if escCount < 2 {
		t.Errorf("expected multiple chunks, got %d escape sequences", escCount)
	}

	if !strings.Contains(output, "m=1") {
		t.Error("output should contain 'more data' flag")
	}
	if !strings.Contains(output, "m=0") {
		t.Error("output should contain 'final chunk' flag")
	}
}

func TestKittyEncoder_Encode_ExactChunkSize(t *testing.T) {
	var buf bytes.Buffer
	enc := NewKittyEncoder(&buf)

	dataSize := (chunkSize * 3) / 4
	data := make([]byte, dataSize)

	err := enc.Encode(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	escCount := strings.Count(output, "\x1b_G")
	if escCount != 1 {
		t.Errorf("expected single chunk for exact size, got %d", escCount)
	}
}

func TestSplitIntoChunks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		size     int
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			size:     10,
			expected: nil,
		},
		{
			name:     "smaller than chunk",
			input:    "hello",
			size:     10,
			expected: []string{"hello"},
		},
		{
			name:     "exact chunk size",
			input:    "hello",
			size:     5,
			expected: []string{"hello"},
		},
		{
			name:     "multiple chunks",
			input:    "hello world",
			size:     5,
			expected: []string{"hello", " worl", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitIntoChunks(tt.input, tt.size)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d chunks, got %d", len(tt.expected), len(result))
			}
			for i, chunk := range result {
				if chunk != tt.expected[i] {
					t.Errorf("chunk %d: expected %q, got %q", i, tt.expected[i], chunk)
				}
			}
		})
	}
}

func TestKittyEncoder_WriteError(t *testing.T) {
	w := &errorWriter{err: bytes.ErrTooLarge}
	enc := NewKittyEncoder(w)

	err := enc.Encode([]byte("test"))
	if err == nil {
		t.Error("expected error from failing writer")
	}
}

type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (int, error) {
	return 0, w.err
}
