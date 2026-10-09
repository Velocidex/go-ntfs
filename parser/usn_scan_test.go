package parser

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// rangeBuffer exposes a byte slice as a single, non sparse range.
type rangeBuffer struct {
	*bytes.Reader
	size int64
}

func (self rangeBuffer) Ranges() []Range {
	return []Range{{Offset: 0, Length: self.size}}
}

func newRangeBuffer(data []byte) rangeBuffer {
	return rangeBuffer{Reader: bytes.NewReader(data), size: int64(len(data))}
}

// putUSNRecordV2 writes a USN_RECORD_V2 at offset and returns its length.
// The record's Usn is its offset in the stream, as it is in $UsnJrnl:$J.
func putUSNRecordV2(buf []byte, offset int64, mft_id uint64, name string) int64 {
	utf16 := make([]byte, 0, 2*len(name))
	for _, c := range name {
		utf16 = append(utf16, byte(c), 0)
	}
	length := (60 + int64(len(utf16)) + 7) &^ 7

	rec := buf[offset : offset+length]
	binary.LittleEndian.PutUint32(rec[0:], uint32(length))
	binary.LittleEndian.PutUint16(rec[4:], 2) // MajorVersion
	binary.LittleEndian.PutUint16(rec[6:], 0) // MinorVersion
	binary.LittleEndian.PutUint64(rec[8:], mft_id|1<<48)
	binary.LittleEndian.PutUint64(rec[16:], 5|1<<48)
	binary.LittleEndian.PutUint64(rec[24:], uint64(offset))

	// 2025-01-01 as a FILETIME
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	filetime := uint64(ts.Unix()+11644473600) * 10000000
	binary.LittleEndian.PutUint64(rec[32:], filetime)

	binary.LittleEndian.PutUint32(rec[40:], 0x100) // USN_REASON_FILE_CREATE
	binary.LittleEndian.PutUint16(rec[56:], uint16(len(utf16)))
	binary.LittleEndian.PutUint16(rec[58:], 60)
	copy(rec[60:], utf16)

	return length
}

func newTestContext(cluster_size int64) *NTFSContext {
	ntfs_ctx := GetNTFSContextFromRawMFT(bytes.NewReader(nil), cluster_size, 0x400)
	options := GetDefaultOptions()
	options.DisableFullPathResolution = true
	ntfs_ctx.SetOptions(options)
	return ntfs_ctx
}

// A record whose RecordLength is a multiple of 256 starts with a zero
// byte. When it follows zero padding, the scan ahead in Next() must
// not lock onto the first non zero byte (offset+1) and must not lose
// the rest of the run.
func TestParseUSNAfterPaddingWithZeroLeadingLength(t *testing.T) {
	data := make([]byte, 0x3000)

	expected := []int64{}
	expected = append(expected, 0)
	putUSNRecordV2(data, 0, 100, "first.txt")

	// 196 UTF-16 bytes of name -> RecordLength 0x100 (low byte 0)
	long_name := strings.Repeat("a", 98)
	expected = append(expected, 0x1000)
	if l := putUSNRecordV2(data, 0x1000, 101, long_name); l != 0x100 {
		t.Fatalf("test setup: record length %#x, want 0x100", l)
	}

	offset := int64(0x1100)
	for i := 0; i < 20; i++ {
		expected = append(expected, offset)
		offset += putUSNRecordV2(data, offset, uint64(200+i), "after.txt")
	}

	ntfs_ctx := newTestContext(0x1000)
	got := []int64{}
	for record := range ParseUSN(context.Background(), ntfs_ctx, newRangeBuffer(data), 0) {
		if int64(record.Usn()) != record.Offset {
			t.Errorf("record at %#x has Usn %#x (%q) - misaligned record emitted",
				record.Offset, record.Usn(), record.Filename())
		}
		got = append(got, record.Offset)
	}

	if len(got) != len(expected) {
		t.Fatalf("got %d records %#x, want %d %#x", len(got), got, len(expected), expected)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("record %d at %#x, want %#x", i, got[i], expected[i])
		}
	}
}

// USN_RECORD_V2 records are 8 byte aligned, so the carver must look at
// every 8 byte boundary, and must not report a record twice when it
// sits in the overlap between two read buffers.
func TestCarveUSNFindsEightByteAlignedRecordsOnce(t *testing.T) {
	const cluster_size = 0x200
	buffer_size := int64(1024 * cluster_size)
	data := make([]byte, 2*buffer_size)

	offsets := []int64{
		0x1008,                             // 8 mod 16
		0x2000,                             // 0 mod 16
		buffer_size - cluster_size + 0x40,  // inside the overlap of buffers 1 and 2
		buffer_size - cluster_size + 0x108, // overlap, 8 mod 16
		buffer_size + 0x2008,               // second buffer, 8 mod 16
	}
	for i, off := range offsets {
		putUSNRecordV2(data, off, uint64(300+i), "carved.txt")
	}

	ntfs_ctx := newTestContext(cluster_size)
	seen := map[int64]int{}
	for item := range CarveUSN(context.Background(), ntfs_ctx,
		bytes.NewReader(data), int64(len(data))) {
		seen[item.DiskOffset]++
	}

	for _, off := range offsets {
		if seen[off] != 1 {
			t.Errorf("record at %#x carved %d times, want 1", off, seen[off])
		}
	}
	if len(seen) != len(offsets) {
		t.Errorf("carved %d distinct offsets %v, want %d", len(seen), seen, len(offsets))
	}
}

// With small clusters a record can be longer than a cluster, so the
// overlap between read buffers must hold the longest record or a
// record that starts just before the overlap is cut short.
func TestCarveUSNReadsLongRecordsAcrossBuffers(t *testing.T) {
	const cluster_size = 0x200
	buffer_size := int64(1024 * cluster_size)
	data := make([]byte, 2*buffer_size)

	long_name := strings.Repeat("b", 255)
	long_offset := buffer_size - cluster_size - 8
	putUSNRecordV2(data, long_offset, 400, long_name)

	ntfs_ctx := newTestContext(cluster_size)
	found := 0
	for item := range CarveUSN(context.Background(), ntfs_ctx,
		bytes.NewReader(data), int64(len(data))) {
		if item.DiskOffset != long_offset {
			t.Errorf("unexpected record at %#x", item.DiskOffset)
			continue
		}
		found++
		if item.Filename() != long_name {
			t.Errorf("record at %#x has a truncated name (%d chars)",
				long_offset, len(item.Filename()))
		}
	}
	if found != 1 {
		t.Errorf("record at %#x carved %d times, want 1", long_offset, found)
	}
}
