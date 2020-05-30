package parser

import (
	"bytes"
	"testing"

	"github.com/alecthomas/assert"
)

func TestReader(t *testing.T) {
	r, _ := NewPagedReader(
		bytes.NewReader([]byte("abcd")),
		3 /* pagesize */, 100 /* cache_size */)

	// Read 1 byte from the end of the buffer.
	buf := make([]byte, 1)
	c, err := r.ReadAt(buf, 3)
	assert.NoError(t, err)
	assert.Equal(t, c, 1)
	assert.Equal(t, buf, []byte{0x64})

	// Read past end (3 byte buffer from offset 3).
	buf = make([]byte, 3)
	c, err = r.ReadAt(buf, 3)
	assert.NoError(t, err)
	assert.Equal(t, c, 3)
	assert.Equal(t, buf, []byte{0x64, 0x00, 0x00})
}
