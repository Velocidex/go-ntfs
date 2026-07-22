package ntfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"www.velocidex.com/golang/go-ntfs/parser"
)

func putU16(b []byte, off int, v uint16) {
	binary.LittleEndian.PutUint16(b[off:], v)
}

func putU32(b []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(b[off:], v)
}

func putU64(b []byte, off int, v uint64) {
	binary.LittleEndian.PutUint64(b[off:], v)
}

func TestInvalidCompressionUnit(t *testing.T) {
	mft := make([]byte, 1024)

	// MFT_ENTRY header
	copy(mft[0:4], []byte("FILE"))
	putU16(mft, 4, 0x30)  // Fixup_offset
	putU16(mft, 6, 0)     // Fixup_count = 0 -> skip fixups
	putU16(mft, 20, 0x38) // Attribute_offset
	putU16(mft, 24, 1024) // Mft_entry_size
	putU16(mft, 28, 1024) // Mft_entry_allocated (>= 0x100 required)

	// NTFS_ATTRIBUTE @ 0x38: non-resident, COMPRESSED $ATTRIBUTE_LIST
	a := 0x38
	putU32(mft, a+0, 32)   // Type = $ATTRIBUTE_LIST
	putU32(mft, a+4, 80)   // Length
	mft[a+8] = 1           // NON-RESIDENT
	putU16(mft, a+12, 1)   // Flags = COMPRESSED
	putU16(mft, a+32, 64)  // Runlist_offset
	putU16(mft, a+34, 64)  // Compression_unit_size = 64 -> 1<<64 == 0
	putU64(mft, a+48, 100) // Actual_size
	putU64(mft, a+56, 100) // Initialized_size

	// Runlist @ a+64: two real runs (>=2 needed to reach the % op)
	rl := a + 64
	mft[rl+0] = 0x11 // length_size=1, run_offset_size=1
	mft[rl+1] = 0x01 // run length = 1
	mft[rl+2] = 0x01 // run offset = +1
	mft[rl+3] = 0x11
	mft[rl+4] = 0x01
	mft[rl+5] = 0x01
	mft[rl+6] = 0x00 // terminator

	fmt.Println("Feeding crafted 1 KiB $MFT record to parser.ParseMFTFile...")
	ch := parser.ParseMFTFile(context.Background(), bytes.NewReader(mft),
		int64(len(mft)), 4096, 1024)
	for range ch {
	}
}

func TestRunlistOOB(t *testing.T) {
	mft := make([]byte, 1024)

	copy(mft[0:4], []byte("FILE"))
	putU16(mft, 4, 0x30)  // Fixup_offset
	putU16(mft, 6, 0)     // Fixup_count = 0
	putU16(mft, 20, 0x38) // Attribute_offset
	putU16(mft, 24, 1024) // Mft_entry_size
	putU16(mft, 28, 1024) // Mft_entry_allocated

	// Non-resident $ATTRIBUTE_LIST whose runlist sits at the very end of
	// the 1024-byte record so ReadAt() short-reads it to 2 bytes.
	a := 0x38
	putU32(mft, a+0, 32)              // Type = $ATTRIBUTE_LIST
	putU32(mft, a+4, 100)             // Length (runlist buffer cap)
	mft[a+8] = 1                      // NON-RESIDENT
	putU16(mft, a+12, 0)              // Flags = 0 (uncompressed)
	putU16(mft, a+32, uint16(1022-a)) // Runlist_offset -> abs 1022
	putU16(mft, a+34, 4)              // Compression_unit_size (irrelevant here)

	// 2-byte truncated runlist at the tail of the record.
	// idx=0x11 -> length_size=1, run_offset_size=1.
	// After idx (off=1) and 1 length byte (off=2), the sign-extension
	// check dereferences buffer[2] on a 2-byte buffer -> OOB.
	mft[1022] = 0x11
	mft[1023] = 0x05

	fmt.Println("Feeding crafted $MFT record to parser.ParseMFTFile...")
	ch := parser.ParseMFTFile(context.Background(), bytes.NewReader(mft),
		int64(len(mft)), 4096, 1024)
	for range ch {
	}
}

func TestFixupOverflow(t *testing.T) {
	img := make([]byte, 64*1024)

	// Boot sector.
	putU16(img, 11, 512)     // Sector_size
	img[13] = 1              // _cluster_size -> ClusterSize = 512
	putU64(img, 40, 64*1024) // _volume_size
	putU64(img, 48, 4)       // _mft_cluster -> MFT @ 4*512 = 2048
	img[64] = 0xF6           // _mft_record_size = -10 -> RecordSize = 1024
	putU16(img, 510, 0xAA55) // boot magic

	// MFT entry 0 @ 2048 with a non-resident $DATA whose runlist maps the
	// $MFT back to itself.
	m := 2048
	copy(img[m:m+4], []byte("FILE"))
	putU16(img, m+4, 0x30)  // Fixup_offset
	putU16(img, m+6, 0)     // Fixup_count = 0 (skip fixups)
	putU16(img, m+20, 0x38) // Attribute_offset
	putU16(img, m+24, 1024) // Mft_entry_size
	putU16(img, m+28, 1024) // Mft_entry_allocated

	a := m + 0x38
	putU32(img, a+0, 128)   // Type = $DATA
	putU32(img, a+4, 72)    // Length
	img[a+8] = 1            // NON-RESIDENT
	putU16(img, a+12, 0)    // Flags = 0
	putU16(img, a+32, 64)   // Runlist_offset
	putU16(img, a+34, 0)    // Compression_unit_size
	putU64(img, a+48, 8192) // Actual_size
	putU64(img, a+56, 8192) // Initialized_size
	img[a+64] = 0x11        // runlist: len=16 clusters, LCN=4
	img[a+65] = 0x10
	img[a+66] = 0x04
	img[a+67] = 0x00

	ntfs, err := parser.GetNTFSContext(bytes.NewReader(img), 0)
	if err != nil {
		fmt.Printf("setup failed: %v\n", err)
		return
	}

	// Crafted 4 KiB INDX page with Fixup_count = 0x8000 at offset 6.
	indx := make([]byte, 0x1000)
	copy(indx[0:4], []byte("INDX"))
	putU16(indx, 4, 0x28)   // Fixup_offset
	putU16(indx, 6, 0x8000) // Fixup_count -> 0x8000*2 wraps to 0 in uint16

	parser.ExtractI30ListFromStream(ntfs, bytes.NewReader(indx), int64(len(indx)))
}
