package ntfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
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

// Open the MFT entry specified by a path name. Walks all directory
// indexes in the path to find the right MFT entry.
func (self *MFT_ENTRY) Open(filename string) (*MFT_ENTRY, error) {
	filename = strings.Replace(filename, "\\", "/", -1)
	components := strings.Split(path.Clean(filename), "/")

	get_path_in_dir := func(component string, dir *MFT_ENTRY) (
		*MFT_ENTRY, error) {

		// NTFS is usually case insensitive.
		component = strings.ToLower(component)
		for _, idx_record := range dir.Dir() {
			item_name := strings.ToLower(
				(&FILE_NAME{idx_record.Get("file")}).Name())
			if item_name == component {
				return self.MFTEntry(
					idx_record.Get("mftReference").
						AsInteger())
			}
		}

		return nil, errors.New("Not found")
	}

	directory := self
	for _, component := range components {
		if component == "" {
			continue
		}
		next, err := get_path_in_dir(component, directory)
		if err != nil {
			return nil, err
		}
		directory = next
	}

	return directory, nil
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

		// This is an $ATTRIBUTE_LIST attribute - append its
		// own attributes to this one.
		if attribute.Get("type").AsInteger() == 32 {
			attr_list, err := attribute.Decode()
			if err == nil {
				result = append(
					result, attr_list.(*ATTRIBUTE_LIST_ENTRY).Attributes()...)
			}
		}

		result = append(result, attribute)

		// Go to the next attribute.
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

// Retrieve the content of the attribute stream specified by id.
func (self *MFT_ENTRY) Data(attr_type, id int64) io.ReaderAt {
	result := &MapReader{}

	cluster_size := self.boot.ClusterSize()
	compression_unit_size := int64(0)
	actual_size := int64(0)

	for _, attr := range self.Attributes() {
		if attr.Get("type").AsInteger() != attr_type {
			continue
		}

		// id of -1 means the first non-named stream.
		if id == -1 {
			if attr.Name() != "" {
				continue
			}

		} else if attr.Get("attribute_id").AsInteger() != id {
			continue
		}

		// The attribute is resident.
		if attr.IsResident() {
			buf := make([]byte, attr.Get("content_size").AsInteger())
			n, _ := attr.Reader().ReadAt(
				buf,
				attr.Offset()+attr.Get("content_offset").AsInteger())
			buf = buf[:n]

			return bytes.NewReader(buf)
		}

		if actual_size == 0 {
			actual_size = attr.Get("actual_size").AsInteger()
		}

		start_vcn := attr.Get("runlist_vcn_start").AsInteger()
		end_vcn := attr.Get("runlist_vcn_end").AsInteger()
		var run io.ReaderAt

		if attr.Get("flags").Get("COMPRESSED").AsInteger() != 0 {
			if compression_unit_size == 0 {
				compression_unit_size = int64(
					1 << uint64(attr.Get(
						"compression_unit_size").
						AsInteger()))
			}
			run = NewCompressedRunReader(
				attr.RunList(), attr, compression_unit_size)
		} else {
			run = NewRunReader(attr.RunList(), attr)
		}

		end_of_run_in_bytes := (end_vcn + 1) * cluster_size
		if end_of_run_in_bytes > actual_size {
			end_of_run_in_bytes = actual_size
		}

		Printf("Adding run from %v to %v\n",
			start_vcn*cluster_size,
			end_of_run_in_bytes)

		result.Runs = append(result.Runs, &GenericRun{
			Offset: start_vcn * cluster_size,
			End:    end_of_run_in_bytes,
			Reader: run,
		})
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
		result = append(result, node.GetRecords()...)
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

type GenericRun struct {
	Offset int64
	End    int64
	Reader io.ReaderAt
}

// Stitch together several different readers mapped at different
// offsets.  In NTFS, a file's data consists of multiple $DATA
// streams, each having the same id. These different streams are
// mapped at different runlist_vcn_start to runlist_vcn_end (VCN =
// Virtual Cluster Number: the cluster number within the file's
// data). This reader combines these different readers into a single
// continuous form.
type MapReader struct {
	// Very simple for now.
	Runs []*GenericRun
}

func (self *MapReader) partialRead(buf []byte, offset int64) (int, error) {
	Printf("MapReader.partialRead %v @ %v\n", len(buf), offset)

	if len(buf) > 0 {
		for _, run := range self.Runs {
			if run.Offset <= offset && offset < run.End {
				available := run.End - offset
				to_read := int64(len(buf))
				if to_read > available {
					to_read = available
				}

				return run.Reader.ReadAt(
					buf[:to_read], offset-run.Offset)
			}
		}
	}
	return 0, io.EOF
}

func (self *MapReader) ReadAt(buf []byte, offset int64) (int, error) {
	to_read := len(buf)
	idx := int(0)

	for to_read > 0 {
		res, err := self.partialRead(buf[idx:], offset+int64(idx))
		if err != nil {
			return idx, err
		}

		to_read -= res
		idx += res
	}

	return idx, nil
}
