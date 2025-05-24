package readers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_UniversalFileReader_CanRead(t *testing.T) {
	r := UniversalFileReader{}
	assert.True(t, r.CanRead("some/file.docx"))
	assert.True(t, r.CanRead("some/file.odt"))
	assert.True(t, r.CanRead("some/file.pdf"))
	assert.True(t, r.CanRead("some/file.txt"))
	assert.True(t, r.CanRead("some/file.xml"))
}

func Test_UniversalFileReader_ReadText(t *testing.T) {
	r := UniversalFileReader{}

	txt, err := r.ReadText("testdata/test.pdf")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(txt))

	txt, err = r.ReadText("testdata/test.docx")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(txt))

	txt, err = r.ReadText("testdata/test.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(txt))

	txt, err = r.ReadText("testdata/test.xml")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(txt))

	txt, err = r.ReadText("testdata/test.odt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(txt))
}
