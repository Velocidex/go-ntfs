package ntfs

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/go-ntfs/parser"
)

type OffsetReader struct {
	offset int64
	fd     io.ReaderAt
}

func (self OffsetReader) ReadAt(buf []byte, offset int64) (int, error) {
	return self.fd.ReadAt(buf, offset-self.offset)
}

func TestUSN(t *testing.T) {
	fd, err := os.Open("usn/sample.bin")
	assert.NoError(t, err)

	reader := &OffsetReader{fd: fd, offset: 0x12a16c30}

	ntfs := &parser.NTFSContext{
		DiskReader: reader,
		Profile:    parser.NewNTFSProfile(),
	}
	record := parser.NewUSN_RECORD(ntfs, reader, 0x12a16c30)

	for record != nil {
		fmt.Printf(record.DebugString())
		record = record.Next(10000)
	}
}
