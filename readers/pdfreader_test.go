package readers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PdfFileReader_CanRead(t *testing.T) {
	r := PdfFileReader{}
	assert.True(t, r.CanRead("some/file.pdf"))
}

func Test_PdfFileReader_ReadText(t *testing.T) {
	r := PdfFileReader{}
	txt, err := r.ReadText("testdata/test.pdf")
	require.NoError(t, err)

	assert.Equal(t, "hello world", strings.TrimSpace(txt))
}
