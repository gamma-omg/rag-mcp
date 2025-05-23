package readers

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/dslipak/pdf"
)

type PdfFileReader struct {
}

func (r *PdfFileReader) CanRead(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".pdf"
}

func (r *PdfFileReader) ReadText(path string) (string, error) {
	reader, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf file: %w", err)
	}

	txt, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("get plain text reader: %w", err)
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(txt)
	if err != nil {
		return "", fmt.Errorf("read text from pdf: %w", err)
	}

	return buf.String(), nil
}
