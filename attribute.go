package ntfs

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"www.velocidex.com/golang/vtypes"
)

type NTFS_ATTRIBUTE struct {
	vtypes.Object

	// The mft entry that owns this attribute.
	mft *MFT_ENTRY

	data io.ReaderAt
}

func (self *NTFS_ATTRIBUTE) Decode() (vtypes.Object, error) {
	switch self.Get("type").AsInteger() {
	case 16:
		res, err := self.Profile().Create(
			"STANDARD_INFORMATION", 0,
			self.Data(), nil)
		return &STANDARD_INFORMATION{res}, err

	case 32:
		res, err := self.Profile().Create(
			"ATTRIBUTE_LIST_ENTRY", 0,
			self.Data(), nil)
		return &ATTRIBUTE_LIST_ENTRY{res, self}, err

	case 48:
		res, err := self.Profile().Create(
			"FILE_NAME", 0,
			self.Data(), nil)
		return &FILE_NAME{res}, err

	default:
		return nil, nil
	}
}

func (self *NTFS_ATTRIBUTE) Name() string {
	length := self.Get("name_length").AsInteger() * 2
	if length > 10000 {
		return "name length too long"
	}
	if length < 0 {
		length = 0
	}

	result, err := self.Profile().Create("String",
		self.Offset()+self.Get("name_offset").AsInteger(),
		self.Reader(), map[string]interface{}{
			"Length":   length,
			"encoding": "utf16",
			"term":     "\x00\x00",
		})
	if err != nil {
		return err.Error()
	}
	return result.AsString()
}

func (self *NTFS_ATTRIBUTE) IsResident() bool {
	return self.Get("resident").AsInteger() == 0
}

func (self *NTFS_ATTRIBUTE) Size() int64 {
	if self.IsResident() {
		return self.Get("content_size").AsInteger()
	}

	return self.Get("actual_size").AsInteger()
}

func (self *NTFS_ATTRIBUTE) DebugString() string {
	result := []string{}

	if self.IsResident() {
		obj, err := self.Profile().Create("NTFS_RESIDENT_ATTRIBUTE",
			self.Offset(), self.Reader(), nil)
		if err == nil {
			result = append(result, obj.DebugString())
		}
	} else {
		result = append(result, self.Object.DebugString())
	}

	length := self.Get("actual_size").AsInteger()
	if length > 100 {
		length = 100
	}
	if length < 0 {
		length = 0
	}

	b := make([]byte, length)
	n, _ := self.Data().ReadAt(b, 0)
	b = b[:n]

	name := self.Name()
	if name != "" {
		result = append(result, "Name: "+name)
	}

	if !self.IsResident() {
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

func (self *NTFS_ATTRIBUTE) Data() io.ReaderAt {
	if self.data != nil {
		return self.data
	}

	if self.IsResident() {
		buf := make([]byte, self.Get("content_size").AsInteger())
		n, _ := self.Reader().ReadAt(
			buf,
			self.Offset()+self.Get("content_offset").AsInteger())
		buf = buf[:n]

		self.data = bytes.NewReader(buf)

		// Stream is compressed
	} else if self.Get("flags").Get("COMPRESSED").AsInteger() != 0 {
		compression_unit_size := int64(1 << uint64(self.Get("compression_unit_size").AsInteger()))
		self.data = NewCompressedRunReader(self.RunList(), self, compression_unit_size)
	} else {
		self.data = NewRunReader(self.RunList(), self)
	}

	return self.data
}

func (self *NTFS_ATTRIBUTE) RunList() []Run {
	result := []Run{}

	attr_length := self.Get("length").AsInteger()
	runlist_offset := self.Offset() +
		self.Get("runlist_offset").AsInteger()

	// Read the entire attribute into memory. This makes it easier
	// to parse the runlist.
	buffer := make([]byte, attr_length)
	n, _ := self.Object.Reader().ReadAt(buffer, runlist_offset)
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

	// The attribute that owns us.
	attribute *NTFS_ATTRIBUTE
}

// Convert the NTFS relative runlist into an absolute run list.
func MakeReaderRuns(runs []Run) []ReaderRun {
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
			})

		} else {
			reader_runs = append(reader_runs, ReaderRun{
				FileOffset:   file_offset,
				TargetOffset: target_offset,
				Length:       run.Length,
			})
		}

		file_offset += run.Length
	}
	return reader_runs
}

func NewCompressedRunReader(runs []Run,
	attr *NTFS_ATTRIBUTE, compression_unit_size int64) *RunReader {
	reader_runs := MakeReaderRuns(runs)

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
			}

			normalized_reader_runs = append(normalized_reader_runs, reader_run)
			run = ReaderRun{
				FileOffset:   reader_run.FileOffset + reader_run.Length,
				TargetOffset: reader_run.TargetOffset + reader_run.Length,
				Length:       run.Length - reader_run.Length,
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
			})
			reader_runs[i+1].Length -= compression_unit_size - run.Length
		}

	}

	Printf("compression_unit_size: %v\nRunlist: %v\n"+
		"Normalized runlist %v to:\nNormal runlist %v\n",
		compression_unit_size, runs,
		reader_runs, normalized_reader_runs)
	return &RunReader{runs: normalized_reader_runs, attribute: attr}
}

func NewRunReader(runs []Run, attr *NTFS_ATTRIBUTE) *RunReader {
	return &RunReader{
		runs:      MakeReaderRuns(runs),
		attribute: attr,
	}
}

// Returns the boot sector.
func (self *RunReader) Boot() *NTFS_BOOT_SECTOR {
	return self.attribute.mft.boot
}

func (self *RunReader) readFromARun(
	run_idx int, buf []byte, run_offset int) (int, error) {

	Printf("readFromARun %v\n", self.runs[run_idx])

	cluster_size := self.Boot().ClusterSize()
	target_offset := self.runs[run_idx].TargetOffset * cluster_size
	is_compressed := self.runs[run_idx].CompressedLength > 0

	if is_compressed {
		decompressed, err := self.runs[run_idx].Decompress(
			self.Boot().Reader(), cluster_size)
		if err != nil {
			return 0, err
		}
		Printf("Decompressed %d from %v\n", len(decompressed), self.runs[run_idx])

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
		to_read := int(self.runs[run_idx].Length*cluster_size) - run_offset
		if len(buf) < to_read {
			to_read = len(buf)
		}

		// The run is sparse - just skip the buffer.
		if target_offset == 0 {
			return to_read, nil

		} else {
			// Run contains data - read it
			// into the buffer.
			n, err := self.Boot().Reader().ReadAt(
				buf[:to_read], target_offset+int64(run_offset))
			return n, err
		}
	}
}

func (self *RunReader) ReadAt(buf []byte, file_offset int64) (
	int, error) {
	buf_idx := 0

	cluster_size := self.Boot().ClusterSize()

	// Find the run which covers the required offset.
	for j := 0; j < len(self.runs) && buf_idx < len(buf); j++ {
		run_file_offset := self.runs[j].FileOffset * cluster_size
		run_length := self.runs[j].Length * cluster_size
		run_end_file_offset := run_file_offset + run_length

		// This run can provide us with some data.
		if run_file_offset <= file_offset &&
			file_offset < run_end_file_offset {

			// The relative offset within the run.
			run_offset := int(file_offset - run_file_offset)

			n, err := self.readFromARun(
				j, buf[buf_idx:], run_offset)
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
		Printf("Could not find runs for offset %d: %v. Clusted size %d\n",
			file_offset, self.runs, cluster_size)
		return buf_idx, io.EOF
	}

	return buf_idx, nil
}

type STANDARD_INFORMATION struct {
	vtypes.Object
}

type FILE_NAME struct {
	vtypes.Object
}

func (self *FILE_NAME) Name() string {
	buf := make([]byte, self.Get("_length_of_name").AsInteger()*2)
	n, _ := self.Reader().ReadAt(buf, self.Get("name").Offset())
	buf = buf[:n]

	return UTF16ToString(buf)
}

func (self *FILE_NAME) DebugString() string {
	return fmt.Sprintf("%s\nName: %s\n", self.Object.DebugString(), self.Name())
}

type INDEX_NODE_HEADER struct {
	vtypes.Object
}

func (self *INDEX_NODE_HEADER) GetRecords() []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	end := self.Get("offset_to_end_index_entry").AsInteger() + self.Offset()
	start := self.Get("offset_to_index_entry").AsInteger() + self.Offset()

	// Need to fit the last entry in - it should be at least size of FILE_NAME
	dummy_record, _ := self.Profile().Create(
		"FILE_NAME", 0, self.Reader(), nil)

	for i := start; i+dummy_record.Size() < end; {
		record, err := self.Profile().Create(
			"INDEX_RECORD_ENTRY", i, self.Reader(), nil)
		if err != nil {
			return result
		}

		result = append(result, &INDEX_RECORD_ENTRY{record})

		// Records have varied sizes.
		size_of_record := record.Get("sizeOfIndexEntry").AsInteger()
		if size_of_record == 0 {
			break
		}

		i += size_of_record
	}

	return result
}

type INDEX_RECORD_ENTRY struct {
	vtypes.Object
}

type STANDARD_INDEX_HEADER struct {
	vtypes.Object
	attr *NTFS_ATTRIBUTE
}

type ATTRIBUTE_LIST_ENTRY struct {
	vtypes.Object
	attr *NTFS_ATTRIBUTE
}

func (self *ATTRIBUTE_LIST_ENTRY) Attributes() []*NTFS_ATTRIBUTE {
	result := []*NTFS_ATTRIBUTE{}

	attribute_size := self.attr.Size()
	offset := int64(0)
	for {
		item, err := self.Profile().Create(
			"ATTRIBUTE_LIST_ENTRY",
			self.Offset()+offset, self.Reader(), nil)
		if err != nil {
			break
		}

		attr_list_entry := &ATTRIBUTE_LIST_ENTRY{item, self.attr}

		if attr_list_entry.Get("mftReference").AsInteger() !=
			self.attr.mft.Get("record_number").AsInteger() {
			attr, err := attr_list_entry.GetAttribute()
			if err != nil {
				break
			}
			result = append(result, attr)
		}
		length := item.Get("length").AsInteger()
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

func (self *ATTRIBUTE_LIST_ENTRY) GetAttribute() (*NTFS_ATTRIBUTE, error) {
	mytype := self.Get("type").AsInteger()
	myid := self.Get("attribute_id").AsInteger()

	mft, err := self.attr.mft.MFTEntry(self.Get("mftReference").AsInteger())
	if err != nil {
		return nil, err
	}
	for _, attr := range mft.Attributes() {
		if attr.Get("type").AsInteger() == mytype &&
			attr.Get("attribute_id").AsInteger() == myid {
			return attr, nil
		}
	}

	return nil, errors.New("No attribute found.")
}

// The STANDARD_INDEX_HEADER has a second layer of fixups.
func NewSTANDARD_INDEX_HEADER(attr *NTFS_ATTRIBUTE, offset int64, length int64) (
	*STANDARD_INDEX_HEADER, error) {
	reader := attr.Data()

	// Read the entire data into a buffer.
	buffer := make([]byte, length)
	n, err := reader.ReadAt(buffer, offset)
	if err != nil {
		return nil, err
	}
	buffer = buffer[:n]

	index, err := attr.Profile().Create(
		"STANDARD_INDEX_HEADER", offset, reader, nil)

	fixup_offset := offset + index.Get("fixup_offset").AsInteger()
	fixup_magic := make([]byte, 2)
	_, err = reader.ReadAt(fixup_magic, fixup_offset)
	if err != nil {
		return nil, err
	}

	fixup_offset += 2

	fixup_table := [][]byte{}
	for i := int64(0); i < index.Get("fixup_count").AsInteger()-1; i++ {
		table_item := make([]byte, 2)
		_, err := reader.ReadAt(table_item, fixup_offset+2*i)
		if err != nil {
			return nil, err
		}
		fixup_table = append(fixup_table, table_item)
	}

	for idx, fixup_value := range fixup_table {
		fixup_offset := (idx+1)*512 - 2
		if buffer[fixup_offset] != fixup_magic[0] ||
			buffer[fixup_offset+1] != fixup_magic[1] {
			return nil, errors.New(fmt.Sprintf("Fixup error with MFT %d",
				index.Get("record_number").AsInteger()))
		}

		// Apply the fixup
		buffer[fixup_offset] = fixup_value[0]
		buffer[fixup_offset+1] = fixup_value[1]
	}

	fixed_up_index, err := attr.Profile().Create(
		"STANDARD_INDEX_HEADER", 0, bytes.NewReader(buffer), nil)
	if err != nil {
		return nil, err
	}

	// Produce a new STANDARD_INDEX_HEADER record with a fixed up
	// page.
	return &STANDARD_INDEX_HEADER{fixed_up_index, attr}, err
}

type DATA struct {
	*NTFS_ATTRIBUTE
}
