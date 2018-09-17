package ntfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"www.velocidex.com/golang/vtypes"
)

// The MFT entry needs to be fixed up.
func _NewMFTEntry(boot *NTFS_BOOT_SECTOR, reader io.ReaderAt, offset int64) (*MFT_ENTRY, error) {
	mft, err := boot.Profile().Create("MFT_ENTRY", offset, reader, nil)
	if err != nil {
		return nil, err
	}

	fixup_offset := offset + mft.Get("fixup_offset").AsInteger()
	fixup_magic := make([]byte, 2)
	_, err = reader.ReadAt(fixup_magic, fixup_offset)
	if err != nil {
		return nil, err
	}

	fixup_offset += 2

	fixup_table := [][]byte{}
	for i := int64(0); i < mft.Get("fixup_count").AsInteger()-1; i++ {
		table_item := make([]byte, 2)
		_, err := reader.ReadAt(table_item, fixup_offset+2*i)
		if err != nil {
			return nil, err
		}
		fixup_table = append(fixup_table, table_item)
	}

	buffer := make([]byte, mft.Get("mft_entry_allocated").AsInteger())
	_, err = reader.ReadAt(buffer, offset)
	if err != nil {
		return nil, err
	}
	for idx, fixup_value := range fixup_table {
		fixup_offset := (idx+1)*512 - 2
		if buffer[fixup_offset] != fixup_magic[0] ||
			buffer[fixup_offset+1] != fixup_magic[1] {
			return nil, errors.New(fmt.Sprintf("Fixup error with MFT %d",
				mft.Get("record_number").AsInteger()))
		}

		// Apply the fixup
		buffer[fixup_offset] = fixup_value[0]
		buffer[fixup_offset+1] = fixup_value[1]
	}

	fixed_up_mft, err := boot.Profile().Create(
		"MFT_ENTRY", 0, bytes.NewReader(buffer), nil)
	if err != nil {
		return nil, err
	}
	return &MFT_ENTRY{fixed_up_mft, mft, boot}, nil
}

// Represents a single MFT entry. This can only be created using
// NTFS_BOOT_SECTOR.MTF().
type MFT_ENTRY struct {
	vtypes.Object

	// The original MFT as read from the disk. Due to fixups, the
	// underlying object we read from is not mapped directly from
	// the disk but we only use the fixed up address space
	// internally - publically we still return the disk address
	// space.
	disk_mft vtypes.Object

	// The boot sector.
	boot *NTFS_BOOT_SECTOR
}

// Convenience method used to extract another MFT entry from the same
// table used by the current entry.
func (self *MFT_ENTRY) MFTEntry(id int64) (*MFT_ENTRY, error) {
	result, err := _NewMFTEntry(self.boot, self.Reader(),
		id*self.boot.RecordSize())
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *MFT_ENTRY) Offset() int64 {
	return self.disk_mft.Offset()
}

func (self *MFT_ENTRY) Reader() io.ReaderAt {
	return self.disk_mft.Reader()
}

func (self *MFT_ENTRY) Attributes() []*NTFS_ATTRIBUTE {
	offset := self.Get("attribute_offset").AsInteger()
	result := []*NTFS_ATTRIBUTE{}

	for {
		// Instantiate the attribute over the fixed up address space.
		item, err := self.Profile().Create(
			"NTFS_ATTRIBUTE", offset, self.Object.Reader(), nil)
		if err != nil {
			return result
		}

		// Reached the end of the MFT entry.
		mft_size := self.Get("mft_entry_size").AsInteger()
		attribute_size := item.Get("length").AsInteger()
		if attribute_size == 0 ||
			attribute_size+offset > mft_size {
			break
		}

		attribute := &NTFS_ATTRIBUTE{
			Object: item,
			mft:    self,
		}

		// $ATTRIBUTE_LIST
		if attribute.Get("type").AsInteger() == 32 {
			attr_list, err := attribute.Decode()
			if err == nil {
				result = append(
					result, attr_list.(*ATTRIBUTE_LIST_ENTRY).Attributes()...)
			}
		}

		result = append(result, attribute)

		offset += item.Get("length").AsInteger()
	}

	return result
}

func indent(input string) string {
	var indented []string
	for _, line := range strings.Split(input, "\n") {
		indented = append(indented, "  "+line)
	}

	return strings.Join(indented, "\n")
}

func (self *MFT_ENTRY) DebugString() string {
	result := []string{}

	for _, field := range self.Fields() {
		field_value := self.Get(field)
		field_desc := fmt.Sprintf(
			"%#03x  %s  %s",
			field_value.Offset(),
			field, field_value.DebugString())

		result = append(result, indent(field_desc))
	}
	sort.Strings(result)

	result = append(result, "Attribute:")
	for _, attr := range self.Attributes() {
		result = append(result, attr.DebugString())

		decoded, err := attr.Decode()
		if err == nil && decoded != nil {
			result = append(result, decoded.DebugString())
		}
	}

	return fmt.Sprintf("[MFT_ENTRY] @ %#0x\n", self.Offset()) +
		strings.Join(result, "\n")
}

// Extract the $STANDARD_INFORMATION attribute from the MFT.
func (self *MFT_ENTRY) StandardInformation() (
	*STANDARD_INFORMATION, error) {
	for _, attr := range self.Attributes() {
		if attr.Get("type").AsInteger() == 16 {
			res, err := attr.Decode()
			return res.(*STANDARD_INFORMATION), err
		}
	}

	return nil, errors.New("$STANDARD_INFORMATION not found!")
}

// Extract the $FILE_NAME attribute from the MFT.
func (self *MFT_ENTRY) FileName() []*FILE_NAME {
	result := []*FILE_NAME{}

	for _, attr := range self.Attributes() {
		if attr.Get("type").AsInteger() == 48 {
			res, err := attr.Decode()
			if err != nil {
				continue
			}

			result = append(result, &FILE_NAME{res})
		}
	}

	return result
}

func (self *MFT_ENTRY) Data() []*DATA {
	result := []*DATA{}

	for _, attr := range self.Attributes() {
		if attr.Get("type").AsInteger() == 128 {
			result = append(result, &DATA{attr})
		}
	}

	return result
}

func (self *MFT_ENTRY) IsDir() bool {
	for _, attr := range self.Attributes() {
		switch attr.Get("type").AsInteger() {
		case 144, 160: // $INDEX_ROOT, $INDEX_ALLOCATION
			return true
		}
	}
	return false
}

func (self *MFT_ENTRY) Dir() []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	for _, node := range self.DirNodes() {
		start := node.Get("offset_to_index_entry").AsInteger() + node.Offset()
		end := node.Get("offset_to_end_index_entry").AsInteger() + node.Offset()

		// Need to fit the last entry in - it should be at least size of FILE_NAME
		for i := start; i+66 < end; {
			record, err := self.Profile().Create(
				"INDEX_RECORD_ENTRY", i, node.Reader(), nil)
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
	}
	return result
}

func (self *MFT_ENTRY) DirNodes() []*INDEX_NODE_HEADER {
	result := []*INDEX_NODE_HEADER{}

	for _, attr := range self.Attributes() {
		switch attr.Get("type").AsInteger() {
		case 144: // $INDEX_ROOT
			index_root, err := self.Profile().Create(
				"INDEX_ROOT", 0, attr.Data(), nil)
			if err == nil {
				result = append(result, &INDEX_NODE_HEADER{
					index_root.Get("node"),
				})
			}

		case 160: // $INDEX_ALLOCATION
			for i := int64(0); i < attr.Size(); i += 0x1000 {
				index_root, err := NewSTANDARD_INDEX_HEADER(
					attr, i, 0x1000)
				if err == nil {
					result = append(
						result, &INDEX_NODE_HEADER{
							index_root.Get("node"),
						},
					)
				}
			}
		}

	}
	return result
}
