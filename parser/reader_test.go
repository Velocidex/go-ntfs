package parser

import (
	"bytes"
	"io"
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
	assert.Error(t, err, io.EOF.Error())
	assert.Equal(t, c, 1)
	assert.Equal(t, buf, []byte{0x64, 0x00, 0x00})
}

func TestRangeReader(t *testing.T) {
	reader := RangeReader{
		runs: []*MappedReader{&MappedReader{
			FileOffset:  10,
			Length:      5,
			ClusterSize: 1,

			// Reader is actually longer than the mapping region.
			Reader: bytes.NewReader([]byte("0123456789")),
		}},
	}

	buf := make([]byte, 100)

	// Should read from 12 to 15 *only* because this is the mapped range.
	c, err := reader.ReadAt(buf, 12)
	assert.NoError(t, err)
	assert.Equal(t, c, 3)
	assert.Equal(t, string(buf[:c]), "234")
}

func TestMappedReader(t *testing.T) {
	reader := MappedReader{
		FileOffset:  10,
		Length:      5,
		ClusterSize: 1,

		// Reader is actually longer than the mapping region.
		Reader: bytes.NewReader([]byte("0123456789")),
	}

	buf := make([]byte, 100)

	// Should read from 12 to 15 *only* because this is the mapped range.
	c, err := reader.ReadAt(buf, 12)
	assert.NoError(t, err)
	assert.Equal(t, c, 3)
	assert.Equal(t, string(buf[:c]), "234")

	// Very short buffer
	buf = make([]byte, 2)

	// Should read from 12 to 14 *only* because this is the mapped range.
	c, err = reader.ReadAt(buf, 12)
	assert.NoError(t, err)
	assert.Equal(t, c, 2)
	assert.Equal(t, string(buf[:c]), "23")

}
