package ntfs

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

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

	case 48:
		res, err := self.Profile().Create(
			"FILE_NAME", 0,
			self.Data(), nil)
		return &FILE_NAME{res}, err

	default:
		return self, nil
	}
}

func (self *NTFS_ATTRIBUTE) Name() string {
	length := self.Get("name_length").AsInteger() * 2
	if length > 1000000 {
		return "name length too long"
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

func (self *NTFS_ATTRIBUTE) Size() int64 {
	if self.Get("resident").AsInteger() == 0 {
		return self.Get("content_size").AsInteger()
	}

	return self.Get("actual_size").AsInteger()
}

func (self *NTFS_ATTRIBUTE) DebugString() string {
	length := self.Get("actual_size").AsInteger()
	if length > 100 {
		length = 100
	}
	b := make([]byte, length)
	n, _ := self.Data().ReadAt(b, 0)
	b = b[:n]

	result := fmt.Sprintf(
		"%s\n ... Attr Name: '%s'\n ... Runlist: "+
			"%v\n ... Data: \n%s\n",
		self.Object.DebugString(),
		self.Name(),
		self.RunList(), hex.Dump(b))

	return result
}

type Run struct {
	FileOffset   int64
	TargetOffset int64
	Length       int64
}

func (self *NTFS_ATTRIBUTE) Data() io.ReaderAt {
	if self.data != nil {
		return self.data
	}

	if self.Get("resident").AsInteger() == 0 {
		buf := make([]byte, self.Get("content_size").AsInteger())
		n, _ := self.Reader().ReadAt(
			buf,
			self.Offset()+self.Get("content_offset").AsInteger())
		buf = buf[:n]

		self.data = bytes.NewReader(buf)
	} else {
		self.data = &RunReader{
			runs:      self.RunList(),
			attribute: self,
		}
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
	run_offset := int64(0)
	file_offset := int64(0)

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

		// Sign extend if the last byte if larger than 0x80.
		var sign byte
		for i := 0; i < 8; i++ {
			if i == run_offset_size &&
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
		run_offset += relative_run_offset

		result = append(result, Run{
			FileOffset:   file_offset,
			TargetOffset: run_offset,
			Length:       run_length,
		})

		file_offset += run_length
	}

	return result
}

// An io.ReaderAt which works off runs.
type RunReader struct {
	runs []Run

	// The attribute that owns us.
	attribute *NTFS_ATTRIBUTE
}

// Returns the boot sector.
func (self *RunReader) Boot() *NTFS_BOOT_SECTOR {
	return self.attribute.mft.boot
}

func (self *RunReader) ReadAt(buf []byte, offset int64) (int, error) {
	buf_idx := 0

	cluster_size := self.Boot().ClusterSize()

	// Find the run which covers the required offset.
	for j := 0; j < len(self.runs) && buf_idx < len(buf); j++ {
		run_file_offset := self.runs[j].FileOffset * cluster_size
		run_length := self.runs[j].Length * cluster_size
		run_end_file_offset := run_file_offset + run_length
		target_offset := self.runs[j].TargetOffset * cluster_size

		if run_file_offset <= offset &&
			offset < run_end_file_offset {

			// Read as much as possible from this run.
			to_read := run_end_file_offset - offset
			if to_read > int64(len(buf))-int64(buf_idx) {
				to_read = int64(len(buf)) - int64(buf_idx)
			}

			n, _ := self.Boot().Reader().ReadAt(
				buf[buf_idx:to_read],
				target_offset+offset-run_file_offset)

			if n == 0 {
				return buf_idx, nil
			}

			buf_idx += n
			offset += int64(n)
		}
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

type INDEX_RECORD_ENTRY struct {
	vtypes.Object
}

type STANDARD_INDEX_HEADER struct {
	vtypes.Object
	attr *NTFS_ATTRIBUTE
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
