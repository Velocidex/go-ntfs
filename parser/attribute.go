package parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

type LimitedReader struct {
	R io.ReaderAt
	N int64
}

func (self LimitedReader) ReadAt(buff []byte, off int64) (int, error) {
	n, err := self.R.ReadAt(buff, off)

	if off+int64(n) > self.N {
		n = int(self.N - off)
	}

	return n, err
}

func (self *NTFS_ATTRIBUTE) Data(ntfs *NTFSContext) io.ReaderAt {
	if self.Resident().Name == "RESIDENT" {
		buf := make([]byte, self.Content_size())
		n, _ := self.Reader.ReadAt(
			buf,
			self.Offset+int64(self.Content_offset()))
		buf = buf[:n]

		return bytes.NewReader(buf)

		// Stream is compressed
	} else if self.Flags().IsSet("COMPRESSED") {
		compression_unit_size := int64(1 << uint64(self.Compression_unit_size()))
		return LimitedReader{
			NewCompressedRunReader(self.RunList(),
				ntfs.Boot.ClusterSize(),
				ntfs.DiskReader,
				compression_unit_size),
			int64(self.Actual_size()),
		}
	} else {
		return LimitedReader{
			NewRunReader(self.RunList(),
				ntfs.Boot.ClusterSize(), ntfs.DiskReader),
			int64(self.Actual_size()),
		}
	}
}

func (self *NTFS_ATTRIBUTE) Name() string {
	length := int64(self.name_length()) * 2
	result := ParseUTF16String(self.Reader,
		self.Offset+int64(self.name_offset()), length)
	return result
}

func (self *NTFS_ATTRIBUTE) IsResident() bool {
	return self.Resident().Value == 0
}

func (self *NTFS_ATTRIBUTE) DataSize() int64 {
	if self.Resident().Name == "RESIDENT" {
		return int64(self.Content_size())
	}

	return int64(self.Actual_size())
}

func (self *NTFS_ATTRIBUTE) PrintStats(ntfs *NTFSContext) string {
	result := []string{}

	if self.Resident().Name == "RESIDENT" {
		obj := self.Profile.NTFS_RESIDENT_ATTRIBUTE(self.Reader,
			self.Offset)
		result = append(result, obj.DebugString())

	} else {
		result = append(result, self.DebugString())
	}

	length := self.Actual_size()
	if length > 100 {
		length = 100
	}
	if length < 0 {
		length = 0
	}

	b := make([]byte, length)
	reader := self.Data(ntfs)
	n, _ := reader.ReadAt(b, 0)
	b = b[:n]

	name := self.Name()
	if name != "" {
		result = append(result, "Name: "+name)
	}

	if self.Resident().Name != "RESIDENT" {
		result = append(result, fmt.Sprintf(
			"Runlist: %v", self.RunList()))
	}

	result = append(result, fmt.Sprintf("Data: \n%s", hex.Dump(b)))
	return strings.Join(result, "\n")
}

type Run struct {
	RelativeUrnOffset int64
	Length            int64
}

func (self *NTFS_ATTRIBUTE) RunList() []Run {
	result := []Run{}

	attr_length := self.Length()
	runlist_offset := self.Offset + int64(self.Runlist_offset())

	// Read the entire attribute into memory. This makes it easier
	// to parse the runlist.
	buffer := make([]byte, attr_length)
	n, _ := self.Reader.ReadAt(buffer, runlist_offset)
	buffer = buffer[:n]

	length_buffer := make([]byte, 8)
	offset_buffer := make([]byte, 8)

	for offset := 0; offset < len(buffer); {
		// Consume the first byte off the stream.
		idx := buffer[offset]
		if idx == 0 {
			break
		}

		length_size := int(idx & 0xF)
		run_offset_size := int(idx >> 4)
		offset += 1

		// Pad out to 8 bytes
		for i := 0; i < 8; i++ {
			if i < length_size {
				length_buffer[i] = buffer[offset]
				offset++
			} else {
				length_buffer[i] = 0
			}
		}

		// Sign extend if the last byte is larger than 0x80.
		var sign byte = 0x00
		for i := 0; i < 8; i++ {
			if i == run_offset_size-1 &&
				buffer[offset]&0x80 != 0 {
				sign = 0xFF
			}

			if i < run_offset_size {
				offset_buffer[i] = buffer[offset]
				offset++
			} else {
				offset_buffer[i] = sign
			}
		}

		relative_run_offset := int64(
			binary.LittleEndian.Uint64(offset_buffer))

		run_length := int64(binary.LittleEndian.Uint64(
			length_buffer))

		result = append(result, Run{
			RelativeUrnOffset: relative_run_offset,
			Length:            run_length,
		})
	}

	return result
}

type ReaderRun struct {
	FileOffset       int64
	TargetOffset     int64
	Length           int64
	CompressedLength int64
	Reader           io.ReaderAt
}

func (self *ReaderRun) Decompress(reader io.ReaderAt, cluster_size int64) ([]byte, error) {
	Printf("Decompress %v\n", self)
	compressed := make([]byte, self.CompressedLength*cluster_size)
	n, err := reader.ReadAt(compressed, self.TargetOffset*cluster_size)
	if err != nil {
		return compressed, err
	}
	compressed = compressed[:n]

	decompressed, err := LZNT1Decompress(compressed)
	return decompressed, err
}

// An io.ReaderAt which works off runs.
type RunReader struct {
	runs []ReaderRun

	cluster_size int64

	// The attribute that owns us.
	//attribute *NTFSAttribute
}

// Convert the NTFS relative runlist into an absolute run list.
func MakeReaderRuns(runs []Run, disk_reader io.ReaderAt) []ReaderRun {
	reader_runs := []ReaderRun{}
	file_offset := int64(0)
	target_offset := int64(0)

	for _, run := range runs {
		target_offset += run.RelativeUrnOffset

		// Sparse run.
		if run.RelativeUrnOffset == 0 {
			reader_runs = append(reader_runs, ReaderRun{
				FileOffset:   file_offset,
				TargetOffset: 0,
				Length:       run.Length,
				Reader:       disk_reader,
			})

		} else {
			reader_runs = append(reader_runs, ReaderRun{
				FileOffset:   file_offset,
				TargetOffset: target_offset,
				Length:       run.Length,
				Reader:       disk_reader,
			})
		}

		file_offset += run.Length
	}
	return reader_runs
}

func NewCompressedRunReader(runs []Run,
	cluster_size int64,
	disk_reader io.ReaderAt,
	compression_unit_size int64) *RunReader {
	reader_runs := MakeReaderRuns(runs, disk_reader)

	normalized_reader_runs := []ReaderRun{}

	// Break runs up into compression units.
	for i := 0; i < len(reader_runs); i++ {
		run := reader_runs[i]
		if run.Length == 0 {
			continue
		}

		if run.Length >= compression_unit_size {
			reader_run := ReaderRun{
				FileOffset:   run.FileOffset,
				TargetOffset: run.TargetOffset,
				Length:       run.Length - run.Length%compression_unit_size,
				Reader:       disk_reader,
			}

			normalized_reader_runs = append(normalized_reader_runs, reader_run)
			run = ReaderRun{
				FileOffset:   reader_run.FileOffset + reader_run.Length,
				TargetOffset: reader_run.TargetOffset + reader_run.Length,
				Length:       run.Length - reader_run.Length,
				Reader:       disk_reader,
			}
		}

		if run.Length == 0 {
			continue
		}

		// This is a compressed run which is part of the
		// compression_unit_size. It is followed by a sparse
		// run. We convert this run to a single compressed run
		// and swallow the sparse run that follows it. eg:
		// [{0 474540 47 0} {47 0 1 0} {48 474588 1213 0} {1261 0 3 0}] Normalizes to:
		// [{0 474540 32 0} {32 474572 16 15} {48 474588 1200 0} {1248 475788 16 13}]
		if i+1 < len(reader_runs) &&
			reader_runs[i+1].Length+run.Length >= compression_unit_size &&
			reader_runs[i+1].TargetOffset == 0 {

			normalized_reader_runs = append(normalized_reader_runs, ReaderRun{
				FileOffset:   run.FileOffset,
				TargetOffset: run.TargetOffset,

				// Take up the entire compression unit
				Length:           compression_unit_size,
				CompressedLength: run.Length,
				Reader:           disk_reader,
			})
			reader_runs[i+1].Length -= compression_unit_size - run.Length
		}

	}

	Printf("compression_unit_size: %v\nRunlist: %v\n"+
		"Normalized runlist %v to:\nNormal runlist %v\n",
		compression_unit_size, runs,
		reader_runs, normalized_reader_runs)
	return &RunReader{runs: normalized_reader_runs, cluster_size: cluster_size}
}

func NewRunReader(runs []Run, cluster_size int64, disk_reader io.ReaderAt) *RunReader {
	return &RunReader{
		runs:         MakeReaderRuns(runs, disk_reader),
		cluster_size: cluster_size,
	}
}

/*
// Returns the boot sector.
func (self *RunReader) Boot() *NTFS_BOOT_SECTOR {
	return self.attribute.mft.Boot
}
*/

func (self *RunReader) readFromARun(
	run_idx int,
	buf []byte,
	cluster_size int64,
	run_offset int) (int, error) {

	//	Printf("readFromARun %v\n", self.runs[run_idx])

	run := self.runs[run_idx]
	target_offset := run.TargetOffset * cluster_size
	is_compressed := run.CompressedLength > 0

	if is_compressed {
		decompressed, err := run.Decompress(run.Reader, cluster_size)
		if err != nil {
			return 0, err
		}
		Printf("Decompressed %d from %v\n", len(decompressed), run)

		i := 0
		for {
			if run_offset >= len(decompressed) ||
				i >= len(buf) {
				return i, nil
			}

			buf[i] = decompressed[run_offset]
			run_offset++
			i++
		}

	} else {
		to_read := int(run.Length*cluster_size) - run_offset
		if len(buf) < to_read {
			to_read = len(buf)
		}

		// The run is sparse - just skip the buffer.
		if target_offset == 0 {
			return to_read, nil

		} else {
			// Run contains data - read it
			// into the buffer.
			n, err := run.Reader.ReadAt(
				buf[:to_read], target_offset+int64(run_offset))
			return n, err
		}
	}
}

func (self *RunReader) ReadAt(buf []byte, file_offset int64) (
	int, error) {
	buf_idx := 0

	// Find the run which covers the required offset.
	for j := 0; j < len(self.runs) && buf_idx < len(buf); j++ {
		run_file_offset := self.runs[j].FileOffset * self.cluster_size
		run_length := self.runs[j].Length * self.cluster_size
		run_end_file_offset := run_file_offset + run_length

		// This run can provide us with some data.
		if run_file_offset <= file_offset &&
			file_offset < run_end_file_offset {

			// The relative offset within the run.
			run_offset := int(file_offset - run_file_offset)

			n, err := self.readFromARun(
				j, buf[buf_idx:], self.cluster_size, run_offset)
			if err != nil {
				Printf("Reading run %v returned error %v\n", self.runs[j], err)
				return buf_idx, err
			}

			if n == 0 {
				Printf("Reading run %v returned no data\n", self.runs[j])
				return buf_idx, io.EOF
			}

			buf_idx += n
			file_offset += int64(n)
		}
	}

	if buf_idx == 0 {
		Printf("Could not find runs for offset %d: %v. Cluster size %d\n",
			file_offset, self.runs, self.cluster_size)
		return buf_idx, io.EOF
	}

	return buf_idx, nil
}

func (self *FILE_NAME) Name() string {
	return ParseUTF16String(self.Reader, self.Offset+self.Profile.Off_FILE_NAME_name,
		int64(self._length_of_name())*2)
}

func (self *INDEX_NODE_HEADER) GetRecords(ntfs *NTFSContext) []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	end := int64(self.Offset_to_end_index_entry()) + self.Offset
	start := int64(self.Offset_to_index_entry()) + self.Offset

	// Need to fit the last entry in - it should be at least size of FILE_NAME
	dummy_record := self.Profile.FILE_NAME(self.Reader, 0)

	for i := start; i+int64(dummy_record.Size()) < end; {
		record := self.Profile.INDEX_RECORD_ENTRY(self.Reader, i)
		result = append(result, record)

		// Records have varied sizes.
		size_of_record := int64(record.SizeOfIndexEntry())
		if size_of_record == 0 {
			break
		}

		i += size_of_record
	}

	return result
}

func (self *ATTRIBUTE_LIST_ENTRY) Attributes(
	ntfs *NTFSContext,
	mft_entry *MFT_ENTRY,
	attr *NTFS_ATTRIBUTE) []*NTFS_ATTRIBUTE {
	result := []*NTFS_ATTRIBUTE{}

	attribute_size := attr.DataSize()
	offset := int64(0)
	for {
		attr_list_entry := self.Profile.ATTRIBUTE_LIST_ENTRY(
			self.Reader, self.Offset+offset)

		// The attribute_list_entry points to a different MFT
		// entry than the one we are working on now. We need
		// to fetch it from there.
		if ntfs.RootMFT != nil &&
			attr_list_entry.MftReference() != uint64(mft_entry.Record_number()) {
			attr, err := attr_list_entry.GetAttribute(ntfs)
			if err != nil {
				break
			}
			result = append(result, attr)
		}
		length := int64(attr_list_entry.Length())
		if length <= 0 {
			break
		}

		offset += length

		if offset >= attribute_size {
			break
		}

	}

	return result
}

func (self *ATTRIBUTE_LIST_ENTRY) GetAttribute(
	ntfs *NTFSContext) (*NTFS_ATTRIBUTE, error) {
	mytype := uint64(self.Type())
	myid := self.Attribute_id()

	mft, err := ntfs.GetMFT(int64(self.MftReference()))
	if err != nil {
		return nil, err
	}
	for _, attr := range mft.EnumerateAttributes(ntfs) {
		if attr.Type().Value == uint64(mytype) &&
			attr.Attribute_id() == uint16(myid) {
			return attr, nil
		}
	}

	return nil, errors.New("No attribute found.")
}

// The STANDARD_INDEX_HEADER has a second layer of fixups.
func DecodeSTANDARD_INDEX_HEADER(
	ntfs *NTFSContext, reader io.ReaderAt, offset int64, length int64) (
	*STANDARD_INDEX_HEADER, error) {

	// Read the entire data into a buffer.
	buffer := make([]byte, length)
	n, err := reader.ReadAt(buffer, offset)
	if err != nil {
		return nil, err
	}
	buffer = buffer[:n]

	index := ntfs.Profile.STANDARD_INDEX_HEADER(reader, offset)
	fixup_offset := offset + int64(index.Fixup_offset())
	fixup_magic := make([]byte, 2)
	_, err = reader.ReadAt(fixup_magic, fixup_offset)
	if err != nil {
		return nil, err
	}

	fixup_offset += 2

	fixup_table := [][]byte{}
	for i := int64(0); i < int64(index.Fixup_count()-1); i++ {
		table_item := make([]byte, 2)
		_, err := reader.ReadAt(table_item, fixup_offset+2*i)
		if err != nil {
			return nil, err
		}
		fixup_table = append(fixup_table, table_item)
	}

	for idx, fixup_value := range fixup_table {
		fixup_offset := (idx+1)*512 - 2
		if fixup_offset+1 >= len(buffer) ||
			buffer[fixup_offset] != fixup_magic[0] ||
			buffer[fixup_offset+1] != fixup_magic[1] {
			return nil, errors.New("Fixup error with MFT")
		}

		// Apply the fixup
		buffer[fixup_offset] = fixup_value[0]
		buffer[fixup_offset+1] = fixup_value[1]
	}

	fixed_up_index := ntfs.Profile.STANDARD_INDEX_HEADER(
		bytes.NewReader(buffer), 0)

	// Produce a new STANDARD_INDEX_HEADER record with a fixed up
	// page.
	return fixed_up_index, nil
}
