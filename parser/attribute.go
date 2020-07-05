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

type Range struct {
	// In bytes
	Offset, Length int64
	IsSparse       bool
}

type RangeReaderAt interface {
	io.ReaderAt

	Ranges() []Range
}

type LimitedReader struct {
	RangeReaderAt
	N int64
}

func (self LimitedReader) ReadAt(buff []byte, off int64) (int, error) {
	n, err := self.RangeReaderAt.ReadAt(buff, off)

	if off+int64(n) > self.N {
		n = int(self.N - off)
	}

	return n, err
}

func (self *NTFS_ATTRIBUTE) Data(ntfs *NTFSContext) io.ReaderAt {
	compression_unit_size := int64(1 << uint64(self.Compression_unit_size()))

	if self.Resident().Name == "RESIDENT" {
		buf := make([]byte, self.Content_size())
		n, _ := self.Reader.ReadAt(
			buf,
			self.Offset+int64(self.Content_offset()))
		buf = buf[:n]

		return bytes.NewReader(buf)

		// Stream is compressed
	} else if self.Flags().IsSet("COMPRESSED") {
		return LimitedReader{
			NewCompressedRangeReader(self.RunList(),
				ntfs.ClusterSize,
				ntfs.DiskReader,
				compression_unit_size),
			int64(self.Actual_size()),
		}
	} else {
		initialized_size := int64(self.Initialized_size())
		runs := []MappedReader{
			MappedReader{
				FileOffset:   0,
				TargetOffset: 0,
				Length:       initialized_size,
				ClusterSize:  1,
				Reader: NewRangeReader(
					self.RunList(),
					ntfs.DiskReader, ntfs.ClusterSize,
					compression_unit_size),
			},
		}

		actual_size := int64(self.Actual_size())
		if actual_size > initialized_size {
			runs = append(runs, MappedReader{
				FileOffset:   initialized_size,
				TargetOffset: 0,
				ClusterSize:  1, // Sizes are in units of bytes
				Length:       actual_size - initialized_size,
				Reader:       &NullReader{}})
		}

		return &RangeReader{runs: runs}
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

// A reader mapping from file space to target space.
type MappedReader struct {
	FileOffset       int64 // Address in the file this range begins
	TargetOffset     int64 // Address in the target reader the range is mapped to.
	Length           int64 // Length of mapping.
	ClusterSize      int64
	CompressedLength int64 // For compressed readers, we need to decompress on read.
	IsSparse         bool
	Reader           io.ReaderAt
}

func (self *MappedReader) ReadAt(buff []byte, off int64) (int, error) {
	return self.Reader.ReadAt(buff, off)
}

func (self MappedReader) DebugString() string {
	return fmt.Sprintf("Mapping %v -> %v with %T\n%v",
		self.FileOffset*self.ClusterSize,
		self.Length*self.ClusterSize+self.FileOffset*self.ClusterSize,
		self.Reader, DebugString(self.Reader, "  "))
}

func (self *MappedReader) Ranges() []Range {
	// If the delegate can tell us more about its ranges then pass
	// it on otherwise we consider the entire run a single range.
	delegate, ok := self.Reader.(RangeReaderAt)
	if ok {
		result := []Range{}
		for _, rng := range delegate.Ranges() {
			rng.Offset += self.FileOffset * self.ClusterSize
			result = append(result, rng)
		}
		return result
	}

	// Ranges are given in bytes.
	return []Range{Range{
		Offset:   self.FileOffset * self.ClusterSize,
		Length:   self.Length * self.ClusterSize,
		IsSparse: self.IsSparse,
	}}
}

func (self *MappedReader) Decompress(reader io.ReaderAt, cluster_size int64) ([]byte, error) {
	Printf("Decompress %v\n", self)
	compressed := make([]byte, self.CompressedLength*cluster_size)
	n, err := reader.ReadAt(compressed, self.TargetOffset*cluster_size)
	if err != nil && err != io.EOF {
		return compressed, err
	}
	compressed = compressed[:n]

	decompressed, err := LZNT1Decompress(compressed)
	return decompressed, err
}

// An io.ReaderAt which works off a sequence of runs. Each run is a
// mapping between filespace to another reader at a specific offset in
// the file address space.
type RangeReader struct {
	runs []MappedReader
}

// Combine the ranges from all the Mapped readers.
func (self *RangeReader) Ranges() []Range {
	result := make([]Range, 0, len(self.runs))
	for _, run := range self.runs {
		result = append(result, run.Ranges()...)
	}
	return result
}

func (self *RangeReader) DebugString() string {
	result := fmt.Sprintf("RangeReader with %v runs:\n", len(self.runs))
	for idx, run := range self.runs {
		result += fmt.Sprintf(
			"Run %v (%T):\n%v\n", idx, run,
			DebugString(run, "  "))
	}
	return result
}

// Convert the NTFS relative runlist into an absolute run list.
func MakeMappedReader(runs []Run, disk_reader io.ReaderAt,
	cluster_size, compression_unit_size int64) []MappedReader {
	reader_runs := []MappedReader{}
	file_offset := int64(0)
	target_offset := int64(0)

	for _, run := range runs {
		target_offset += run.RelativeUrnOffset

		// Sparse run.
		if run.RelativeUrnOffset == 0 {
			reader_runs = append(reader_runs, MappedReader{
				FileOffset:   file_offset,
				TargetOffset: 0,
				Length:       run.Length,
				ClusterSize:  cluster_size,
				IsSparse:     true,
				Reader:       &NullReader{},
			})

			// Compressed run
		} else if run.Length < compression_unit_size {
			reader_runs = append(reader_runs, MappedReader{
				FileOffset:   file_offset,
				TargetOffset: target_offset,
				Length:       run.Length,
				ClusterSize:  cluster_size,
				IsSparse:     false,
				Reader:       disk_reader,
			})
			file_offset += compression_unit_size

		} else {
			reader_runs = append(reader_runs, MappedReader{
				FileOffset:   file_offset,
				TargetOffset: target_offset,
				Length:       run.Length,
				ClusterSize:  cluster_size,
				IsSparse:     false,
				Reader:       disk_reader,
			})
			file_offset += run.Length + 1
		}

	}
	return reader_runs
}

func NewCompressedRangeReader(runs []Run,
	cluster_size int64,
	disk_reader io.ReaderAt,
	compression_unit_size int64) *RangeReader {
	reader_runs := MakeMappedReader(
		runs, disk_reader, cluster_size, compression_unit_size)

	normalized_reader_runs := []MappedReader{}

	// Break runs up into compression units.
	for i := 0; i < len(reader_runs); i++ {
		run := reader_runs[i]
		if run.Length == 0 {
			continue
		}

		if run.Length >= compression_unit_size {
			reader_run := MappedReader{
				FileOffset:   run.FileOffset,
				TargetOffset: run.TargetOffset,
				Length:       run.Length - run.Length%compression_unit_size,
				IsSparse:     false,
				ClusterSize:  cluster_size,
				Reader:       disk_reader,
			}

			normalized_reader_runs = append(normalized_reader_runs, reader_run)
			run = MappedReader{
				FileOffset:   reader_run.FileOffset + reader_run.Length,
				TargetOffset: reader_run.TargetOffset + reader_run.Length,
				Length:       run.Length - reader_run.Length,
				IsSparse:     false,
				ClusterSize:  cluster_size,
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

			normalized_reader_runs = append(normalized_reader_runs, MappedReader{
				FileOffset:   run.FileOffset,
				TargetOffset: run.TargetOffset,

				// Take up the entire compression unit
				Length:           compression_unit_size,
				CompressedLength: run.Length,
				ClusterSize:      cluster_size,
				IsSparse:         false,
				Reader:           disk_reader,
			})
			reader_runs[i+1].Length -= compression_unit_size - run.Length
		}

	}

	Printf("compression_unit_size: %v\nRunlist: %v\n"+
		"Normalized runlist %v to:\nNormal runlist %v\n",
		compression_unit_size, runs,
		reader_runs, normalized_reader_runs)
	return &RangeReader{runs: normalized_reader_runs}
}

func NewRangeReader(runs []Run, disk_reader io.ReaderAt,
	cluster_size, compression_unit_size int64) *RangeReader {
	return &RangeReader{
		runs: MakeMappedReader(
			runs, disk_reader, cluster_size, compression_unit_size),
	}
}

func (self *RangeReader) readFromARun(
	run_idx int,
	buf []byte,
	run_offset int) (int, error) {

	//	Printf("readFromARun %v\n", self.runs[run_idx])

	run := self.runs[run_idx]
	target_offset := run.TargetOffset * run.ClusterSize
	is_compressed := run.CompressedLength > 0

	if is_compressed {
		decompressed, err := run.Decompress(run.Reader, run.ClusterSize)
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
		to_read := run.Length*run.ClusterSize - int64(run_offset)
		if int64(len(buf)) < to_read {
			to_read = int64(len(buf))
		}

		// Run contains data - read it
		// into the buffer.
		n, err := run.Reader.ReadAt(
			buf[:to_read], target_offset+int64(run_offset))
		return n, err
	}
}

func (self *RangeReader) ReadAt(buf []byte, file_offset int64) (
	int, error) {
	buf_idx := 0

	// Find the run which covers the required offset.
	for j := 0; j < len(self.runs) && buf_idx < len(buf); j++ {
		run := self.runs[j]

		run_file_offset := run.FileOffset * run.ClusterSize
		run_length := run.Length * run.ClusterSize
		run_end_file_offset := run_file_offset + run_length

		// This run can provide us with some data.
		if run_file_offset <= file_offset &&
			file_offset < run_end_file_offset {

			// The relative offset within the run.
			run_offset := int(file_offset - run_file_offset)

			n, err := self.readFromARun(j, buf[buf_idx:], run_offset)
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
			file_offset, self.runs, self.runs[0].ClusterSize)
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
	if err != nil && err != io.EOF {
		return nil, err
	}
	buffer = buffer[:n]

	index := ntfs.Profile.STANDARD_INDEX_HEADER(reader, offset)

	fixup_offset := offset + int64(index.Fixup_offset())
	fixup_count := index.Fixup_count()
	if fixup_count > 0 {
		fixup_table := make([]byte, fixup_count*2)
		_, err = reader.ReadAt(fixup_table, fixup_offset)
		if err != nil && err != io.EOF {
			return nil, err
		}

		fixup_magic := []byte{fixup_table[0], fixup_table[1]}
		sector_idx := 0
		for idx := 2; idx < len(fixup_table); idx += 2 {
			fixup_offset := (sector_idx+1)*512 - 2
			if fixup_offset+1 >= len(buffer) ||
				buffer[fixup_offset] != fixup_magic[0] ||
				buffer[fixup_offset+1] != fixup_magic[1] {
				return nil, errors.New("Fixup error with MFT")
			}

			// Apply the fixup
			buffer[fixup_offset] = fixup_table[idx]
			buffer[fixup_offset+1] = fixup_table[idx+1]
			sector_idx += 1
		}
	}

	fixed_up_index := ntfs.Profile.STANDARD_INDEX_HEADER(
		bytes.NewReader(buffer), 0)

	// Produce a new STANDARD_INDEX_HEADER record with a fixed up
	// page.
	return fixed_up_index, nil
}

type NullReader struct{}

func (self *NullReader) ReadAt(buf []byte, offset int64) (int, error) {
	for i := 0; i < len(buf); i++ {
		buf[i] = 0
	}

	return len(buf), nil
}
