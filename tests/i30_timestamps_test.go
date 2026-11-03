package ntfs

import (
	"bytes"
	"testing"
	"time"

	"www.velocidex.com/golang/go-ntfs/parser"
)

// winFileTime converts a unix epoch second to a Windows FILETIME.
func winFileTime(unix int64) uint64 {
	return uint64(unix+11644473600) * 10000000
}

// buildINDXPage returns a single 4 KiB $I30 page holding exactly one
// INDEX_RECORD_ENTRY whose four $FILE_NAME timestamps are all distinct,
// so that any mislabelling in the decoder is visible.
func buildINDXPage(created, fileModified, mftModified, fileAccessed int64) []byte {
	page := make([]byte, 0x1000)

	copy(page[0:4], []byte("INDX"))
	putU16(page, 4, 0x28) // Fixup_offset
	putU16(page, 6, 0)    // Fixup_count = 0 -> fixups skipped

	// INDEX_NODE_HEADER lives at 24.
	const node = 24
	const entrySize = 88
	putU32(page, node+0, 16)           // Offset_to_index_entry
	putU32(page, node+4, 16+entrySize) // Offset_to_end_index_entry
	putU32(page, node+8, 16+entrySize) // SizeOfEntriesAlloc

	// INDEX_RECORD_ENTRY at node+16.
	e := node + 16
	putU64(page, e+0, 42)        // MftReference
	putU16(page, e+6, 1)         // Seq_num
	putU16(page, e+8, entrySize) // SizeOfIndexEntry
	putU16(page, e+10, 16)       // FilenameOffset
	putU32(page, e+12, 0)        // Flags

	// FILE_NAME at e+16.
	f := e + 16
	putU64(page, f+0, 5)                          // parent MftReference
	putU64(page, f+8, winFileTime(created))       // Created
	putU64(page, f+16, winFileTime(fileModified)) // File_modified
	putU64(page, f+24, winFileTime(mftModified))  // Mft_modified
	putU64(page, f+32, winFileTime(fileAccessed)) // File_accessed
	putU64(page, f+40, 4096)                      // Allocated_size
	putU64(page, f+48, 1234)                      // FilenameSize (real size)
	putU32(page, f+56, 32)                        // Flags = ARCHIVE
	page[f+64] = 1                                // _length_of_name
	page[f+65] = 1                                // NameType = Win32
	putU16(page, f+66, uint16('a'))               // name, UTF-16LE

	return page
}

func TestI30TimestampMapping(t *testing.T) {
	const (
		created      = int64(1609459200) // 2021-01-01
		fileModified = int64(1643760000) // 2022-02-02
		mftModified  = int64(1677801600) // 2023-03-03
		fileAccessed = int64(1712188800) // 2024-04-04
	)

	page := buildINDXPage(created, fileModified, mftModified, fileAccessed)

	// Exactly the context the "ntfs i30" command builds (bin/i30.go).
	ntfs := &parser.NTFSContext{Profile: parser.NewNTFSProfile()}

	infos := parser.ExtractI30ListFromStream(
		ntfs, bytes.NewReader(page), int64(len(page)))

	var info *parser.FileInfo
	for _, i := range infos {
		if !i.IsSlack {
			info = i
			break
		}
	}
	if info == nil {
		t.Fatalf("no non-slack INDX entry decoded (got %d entries)", len(infos))
	}

	check := func(label string, got time.Time, wantUnix int64) {
		t.Helper()
		if got.Unix() != wantUnix {
			t.Errorf("%s = %v (unix %d), want unix %d (%v)",
				label, got.UTC(), got.Unix(), wantUnix,
				time.Unix(wantUnix, 0).UTC())
		}
	}

	// A FileInfo built from the MFT fills these from $STANDARD_INFORMATION
	// as Mtime=File_altered, Atime=File_accessed, Ctime=Mft_altered,
	// Btime=Create_time (parser/easy.go). The $FILE_NAME equivalents must
	// land in the same four fields.
	check("Btime", info.Btime, created)
	check("Mtime", info.Mtime, fileModified)
	check("Ctime", info.Ctime, mftModified)
	check("Atime", info.Atime, fileAccessed)
}
