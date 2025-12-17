package display

import (
	"encoding/base64"
	"fmt"
	"io"
)

const (
	escapeStart = "\x1b_G"
	escapeEnd   = "\x1b\\"
	chunkSize   = 4096
)

type KittyEncoder struct {
	out io.Writer
}

func NewKittyEncoder(out io.Writer) *KittyEncoder {
	return &KittyEncoder{out: out}
}

func (e *KittyEncoder) Encode(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	if len(encoded) <= chunkSize {
		return e.writeSingle(encoded)
	}

	return e.writeChunked(encoded)
}

func (e *KittyEncoder) writeSingle(encoded string) error {
	_, err := fmt.Fprintf(e.out, "%sa=T,f=100,q=2;%s%s", escapeStart, encoded, escapeEnd)
	return err
}

func (e *KittyEncoder) writeChunked(encoded string) error {
	chunks := splitIntoChunks(encoded, chunkSize)

	for i, chunk := range chunks {
		isFirst := i == 0
		isLast := i == len(chunks)-1

		var params string
		switch {
		case isFirst:
			params = "a=T,f=100,q=2,m=1"
		case isLast:
			params = "m=0"
		default:
			params = "m=1"
		}

		if _, err := fmt.Fprintf(e.out, "%s%s;%s%s", escapeStart, params, chunk, escapeEnd); err != nil {
			return err
		}
	}

	return nil
}

func splitIntoChunks(s string, size int) []string {
	var chunks []string
	for len(s) > 0 {
		if len(s) < size {
			size = len(s)
		}
		chunks = append(chunks, s[:size])
		s = s[size:]
	}
	return chunks
}
