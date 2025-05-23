package readers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_TxtFileReader_CanRead(t *testing.T) {
	r := TxtFileReader{}
	assert.True(t, r.CanRead("some/file.txt"))
}

func Test_TxtFileReader_ReadText(t *testing.T) {
	r := TxtFileReader{}
	txt, err := r.ReadText("testdata/test.txt")
	require.NoError(t, err)

	assert.Equal(t, "hello world", txt)
}
