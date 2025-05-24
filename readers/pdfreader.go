package readers

import (
	"fmt"
	"path/filepath"

	"code.sajari.com/docconv/v2"
)

type PdfFileReader struct {
}

func (r *PdfFileReader) CanRead(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".pdf"
}

func (r *PdfFileReader) ReadText(path string) (string, error) {
	res, err := docconv.ConvertPath(path)
	if err != nil {
		return "", fmt.Errorf("failed to read pdf document: %w", err)
	}

	return res.Body, nil
}
