package parser

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/alecthomas/assert"
)

func TestReader(t *testing.T) {
	r, _ := NewPagedReader(
		bytes.NewReader([]byte("abcde")),
		3 /* pagesize */, 100 /* cache_size */)

	// Read second byte
	buf := make([]byte, 1)
	c, err := r.ReadAt(buf, 1)
	assert.NoError(t, err)
	assert.Equal(t, c, 1)
	assert.Equal(t, buf, []byte{'b'})

	// Read 1 byte from the end of the buffer. This is a partial page
	// so we return the full read with the EOF
	buf = make([]byte, 1)
	c, err = r.ReadAt(buf, 3)
	assert.NoError(t, err)
	assert.Equal(t, c, 1)
	assert.Equal(t, buf, []byte{'d'})

	// Read past end (3 byte buffer from offset 3). Buffer will be
	// padded to pagesize and return EOF.
	buf = make([]byte, 3)
	c, err = r.ReadAt(buf, 3)
	assert.NoError(t, err)
	assert.Equal(t, c, 3)
	assert.Equal(t, buf, []byte{'d', 'e', 0x00})

	// Read far outside the buffer. Return 0 bytes and EOF
	buf = make([]byte, 3)
	c, err = r.ReadAt(buf, 30)
	assert.True(t, errors.Is(err, io.EOF))
	assert.Equal(t, c, 0)

	// Do a big read (Larger than 10 pages will bypass the lru and
	// read directly)
	buf = make([]byte, 300)
	c, err = r.ReadAt(buf, 1)
	assert.NoError(t, err)
	assert.Equal(t, c, 300)

	// Do a medium read (smaller than 10 pages)
	buf = make([]byte, 3*5)
	c, err = r.ReadAt(buf, 1)
	assert.NoError(t, err)
	assert.Equal(t, c, 15)

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
