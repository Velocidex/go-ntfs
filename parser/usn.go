package parser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Parse USN records
// https://docs.microsoft.com/en-us/windows/win32/api/winioctl/ns-winioctl-usn_record_v2

type USN_RECORD struct {
	*USN_RECORD_V2
	context *NTFSContext
}

func (self *USN_RECORD) DebugString() string {
	result := self.USN_RECORD_V2.DebugString()
	result += fmt.Sprintf("  Filename: %v\n", self.Filename())
	return result
}

func (self *USN_RECORD) Filename() string {
	return ParseUTF16String(self.Reader,
		self.Offset+int64(self.FileNameOffset()),
		CapInt64(int64(self.FileNameLength()), MAX_FILENAME_LENGTH))
}

func (self *USN_RECORD) Validate() bool {
	return self.Usn() > 0 && self.RecordLength() != 0
}

func (self *USN_RECORD) Next(max_offset int64) *USN_RECORD {
	length := int64(self.RecordLength())

	// Record length should be reasonable and 64 bit aligned.
	if length > 0 && length < 1024 &&
		(self.Offset+length)%8 == 0 {

		result := NewUSN_RECORD(self.context, self.Reader, self.Offset+length)
		// Only return if the record is valid
		if result.Validate() {
			return result
		}
	}

	// Sometimes there is a sequence of null bytes after a record
	// and before the next record. If the next record is not
	// immediately after the previous record we scan ahead a bit
	// to try to find it.

	// Scan ahead trying to find the next record. We search for
	// the first non-zero byte and try to instantiate a record
	// over it. If the record is valid we return it.
	for offset := self.Offset + length; offset <= max_offset; {
		to_read := max_offset - offset
		data := make([]byte, CapInt64(to_read, MAX_USN_RECORD_SCAN_SIZE))

		n, err := self.Reader.ReadAt(data, offset)
		if n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
			return nil
		}

		// scan the buffer for the first non zero byte.
		for i := 0; i < n; i++ {
			if data[i] != 0 {
				result := NewUSN_RECORD(
					self.context, self.Reader, offset+int64(i))
				if result.Validate() {
					return result
				}
			}
		}

		offset += int64(len(data))
	}

	return nil
}

func (self *USN_RECORD) Links() []string {
	return self._Links(DefaultMaxLinks)
}

func (self *USN_RECORD) _Links(depth int) []string {
	if self.context.GetOptions().DisableFullPathResolution {
		return []string{self.Filename()}
	}

	// Since this record could have meant a file deletion event
	// then resolving the actual MFT entry to a full path is less
	// reliable. It is more reliable to resolve the parent path,
	// and then add the USN record name to it.
	parent_mft_id := self.USN_RECORD_V2.ParentFileReferenceNumberID()
	parent_mft_sequence := uint16(
		self.USN_RECORD_V2.ParentFileReferenceNumberSequence())

	// Make sure the parent has the correct sequence to prevent
	// nonsensical paths.
	parent_mft_entry, err := self.context.mft_summary_cache.GetSummary(
		parent_mft_id, parent_mft_sequence)
	if err != nil {
		return []string{fmt.Sprintf("<Err>\\<Parent %v Error %v>\\%v",
			parent_mft_id, err, self.Filename())}
	}

	if uint64(parent_mft_entry.Sequence) != uint64(parent_mft_sequence) {
		return []string{fmt.Sprintf("<Err>\\<Parent %v-%v need %v>\\%v",
			parent_mft_id, parent_mft_entry.Sequence, parent_mft_sequence,
			self.Filename())}
	}

	components := self.context.full_path_resolver.GetHardLinks(
		uint64(parent_mft_id), parent_mft_sequence, DefaultMaxLinks)
	result := make([]string, 0, len(components))
	for _, l := range components {
		l = append(l, self.Filename())
		result = append(result, strings.Join(l, "\\"))
	}
	return result
}

// Resolve the file to a full path
func (self *USN_RECORD) FullPath() string {
	res := self._Links(1)
	if len(res) == 0 {
		return ""
	}
	return res[0]
}

func (self *USN_RECORD) Reason() []string {
	return self.USN_RECORD_V2.Reason().Values()
}

func (self *USN_RECORD) FileAttributes() []string {
	return self.USN_RECORD_V2.FileAttributes().Values()
}

func (self *USN_RECORD) SourceInfo() []string {
	return self.USN_RECORD_V2.SourceInfo().Values()
}

func NewUSN_RECORD(ntfs *NTFSContext, reader io.ReaderAt, offset int64) *USN_RECORD {
	return &USN_RECORD{
		USN_RECORD_V2: ntfs.Profile.USN_RECORD_V2(reader, offset),
		context:       ntfs,
	}
}

func getUSNStream(ntfs_ctx *NTFSContext) (mft_id int64, attr_id uint16, attr_name string, err error) {
	dir, err := ntfs_ctx.GetMFT(5)
	if err != nil {
		return 0, 0, "", err
	}

	// Open the USN file from the root of the filesystem.
	mft_entry, err := dir.Open(ntfs_ctx, "$Extend\\$UsnJrnl")
	if err != nil {
		return 0, 0, "", errors.New("Can not open path")
	}

	// Find the attribute we need.
	for _, attr := range mft_entry.EnumerateAttributes(ntfs_ctx) {
		name := attr.Name()
		if attr.Type().Value == ATTR_TYPE_DATA && name == "$J" {
			return int64(mft_entry.Record_number()),
				attr.Attribute_id(), name, nil
		}
	}
	return 0, 0, "", errors.New("Can not find $Extend\\$UsnJrnl:$J")
}

func OpenUSNStream(ntfs_ctx *NTFSContext) (RangeReaderAt, error) {
	mft_id, attr_id, attr_name, err := getUSNStream(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	mft_entry, err := ntfs_ctx.GetMFT(mft_id)
	if err != nil {
		return nil, err
	}

	return OpenStream(ntfs_ctx, mft_entry, 128, attr_id, attr_name)
}

// Returns a channel which will send USN records on. We start parsing
// at the start of the file and continue until the end.
func ParseUSN(ctx context.Context,
	ntfs_ctx *NTFSContext,
	usn_stream RangeReaderAt,
	starting_offset int64) chan *USN_RECORD {

	output := make(chan *USN_RECORD)

	go func() {
		defer close(output)

		count := 0
		for _, rng := range usn_stream.Ranges() {
			run_end := rng.Offset + rng.Length
			if rng.IsSparse {
				continue
			}

			if starting_offset > run_end {
				count++
				continue
			}

			for record := NewUSN_RECORD(ntfs_ctx, usn_stream, rng.Offset); record != nil; record = record.Next(run_end) {
				if record.Offset < starting_offset {
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output <- record:
				}
			}
		}
	}()

	return output
}

// Find the last USN record of the file.
func getLastUSN(ctx context.Context, ntfs_ctx *NTFSContext) (record *USN_RECORD, err error) {
	mft_id, attr_id, attr_name, err := getUSNStream(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	mft_entry, err := ntfs_ctx.GetMFT(mft_id)
	if err != nil {
		return nil, err
	}

	data, err := OpenStream(ntfs_ctx, mft_entry, 128, attr_id, attr_name)
	if err != nil {
		return nil, err
	}

	// Get the last range
	ranges := []Range{}
	for _, rng := range data.Ranges() {
		if !rng.IsSparse {
			ranges = append(ranges, rng)
		}
	}

	if len(ranges) == 0 {
		return nil, errors.New("No ranges found!")
	}

	last_range := ranges[len(ranges)-1]
	var result *USN_RECORD

	DebugPrint("Staring to parse USN in offset for seek %v\n", last_range.Offset)
	count := 0
	usn_stream, err := OpenUSNStream(ntfs_ctx)
	if err != nil {
		return nil, err
	}

	for record := range ParseUSN(
		ctx, ntfs_ctx, usn_stream, last_range.Offset) {
		result = record
		count++
	}
	DebugPrint("Parsed %v USN records\n", count)

	if result == nil {
		return nil, errors.New("No ranges found!")
	}
	return result, nil
}

func WatchUSN(ctx context.Context, ntfs_ctx *NTFSContext, period int) chan *USN_RECORD {
	output := make(chan *USN_RECORD)

	// Default 30 second watch frequency.
	if period == 0 {
		period = 30
	}

	go func() {
		defer close(output)

		start_offset := int64(0)

		for {
			usn, err := getLastUSN(ctx, ntfs_ctx)
			if err == nil && usn != nil {
				start_offset = usn.Offset
				break
			}

			// Keep waiting here until we are able to get the last USN entry.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(period) * time.Second):
			}
		}

		for {
			count := 0
			DebugPrint("Checking usn from %#08x\n", start_offset)

			// Purge all caching in the context before we read it so
			// we always get fresh data.
			ntfs_ctx.Purge()

			usn_stream, err := OpenUSNStream(ntfs_ctx)
			if err != nil {
				return
			}

			for record := range ParseUSN(
				ctx, ntfs_ctx, usn_stream, start_offset) {
				if record.Offset > start_offset {
					select {
					case <-ctx.Done():
						return

					case output <- record:
						count++
					}
					start_offset = record.Offset
				}

			}
			DebugPrint("Emitted %v events\n", count)

			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second * time.Duration(period)):
			}
		}
	}()

	return output
}

type USNCarvedRecord struct {
	*USN_RECORD
	DiskOffset int64
}

func CarveUSN(ctx context.Context,
	ntfs_ctx *NTFSContext,
	stream io.ReaderAt,
	size int64) chan *USNCarvedRecord {
	output := make(chan *USNCarvedRecord)

	go func() {
		defer close(output)

		cluster_size := ntfs_ctx.ClusterSize
		if cluster_size == 0 {
			cluster_size = 0x1000
		}

		buffer_size := 1024 * cluster_size

		buffer := make([]byte, buffer_size)

		now := time.Now()

		// Overlap buffers in case an entry is split
		for i := int64(0); i < size; i += buffer_size - cluster_size {
			select {
			case <-ctx.Done():
				return
			default:
			}

			DebugPrint("%v: Reading buffer length %v at %v in %v\n",
				time.Now(), len(buffer), i, time.Now().Sub(now))

			now = time.Now()
			n, err := stream.ReadAt(buffer, i)
			if err != nil && err != io.EOF {
				return
			}

			if n < 64 {
				return
			}

			buf_reader := bytes.NewReader(buffer[:n])

			// We assume that entries are aligned to 0x10 at least.
			for j := int64(0); j < int64(n)-0x10; j += 0x10 {

				// MajorVersion must be 2 and MinorVersion 0. This is
				// a quick check that should eliminate most of the
				// false positives. We check more carefully below.
				if buffer[j+4] != '\x02' ||
					buffer[j+5] != '\x00' ||
					buffer[j+6] != '\x00' ||
					buffer[j+7] != '\x00' {
					continue
				}

				record, ok := testUSNEntry(ntfs_ctx, buf_reader, j)
				if !ok {
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output <- &USNCarvedRecord{
					USN_RECORD: record,
					DiskOffset: j + i,
				}:
				}
			}
		}
	}()

	return output
}

var (
	year2020 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	year2040 = time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
)

func testUSNEntry(ntfs_ctx *NTFSContext,
	reader io.ReaderAt, offset int64) (*USN_RECORD, bool) {

	record := ntfs_ctx.Profile.USN_RECORD_V2(reader, offset)
	record_length := record.RecordLength()
	if record_length < 64 || record_length > 1024 {
		return nil, false
	}

	if record.FileNameOffset() > 255 ||
		record.FileNameLength() > 255 {
		return nil, false
	}

	// Check the the time is reasonable
	ts := record.TimeStamp()
	if ts.Before(year2020) || ts.After(year2040) {
		return nil, false
	}

	return &USN_RECORD{
		USN_RECORD_V2: record,
		context:       ntfs_ctx,
	}, true
}
