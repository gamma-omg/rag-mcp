package readers

import (
	"fmt"
	"os"
	"path/filepath"
)

type TxtFileReader struct{}

func (r *TxtFileReader) CanRead(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".txt"
}

func (r *TxtFileReader) ReadText(path string) (string, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading text file: %w", err)
	}

	return string(buf), nil
}
