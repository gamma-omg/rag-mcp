package readers

import (
	"fmt"
	"path/filepath"

	"code.sajari.com/docconv/v2"
)

type UniversalFileReader struct {
}

func (r *UniversalFileReader) CanRead(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".txt" || ext == ".docx" || ext == ".odt" || ext == ".pdf" || ext == ".xml"
}

func (r *UniversalFileReader) ReadText(path string) (string, error) {
	res, err := docconv.ConvertPath(path)
	if err != nil {
		return "", fmt.Errorf("failed to read document: %w", err)
	}

	return res.Body, nil
}
