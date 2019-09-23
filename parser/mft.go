package parser

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

func (self *MFT_ENTRY) EnumerateAttributes(ntfs *NTFSContext) []*NTFS_ATTRIBUTE {
	offset := int64(self.Attribute_offset())
	result := []*NTFS_ATTRIBUTE{}

	for {
		// Instantiate the attribute over the fixed up address space.
		attribute := self.Profile.NTFS_ATTRIBUTE(
			self.Reader, offset)

		// Reached the end of the MFT entry.
		mft_size := int64(self.Mft_entry_size())
		attribute_size := int64(attribute.Length())
		if attribute_size == 0 ||
			attribute_size+offset > mft_size {
			break
		}

		Printf(attribute.DebugString())

		// This is an $ATTRIBUTE_LIST attribute - append its
		// own attributes to this one.
		if attribute.Type().Name == "$ATTRIBUTE_LIST" {
			attr_list := self.Profile.ATTRIBUTE_LIST_ENTRY(
				attribute.Data(ntfs), 0)

			attr_list_members := attr_list.Attributes(
				ntfs, self, attribute)

			result = append(result, attr_list_members...)
		}

		result = append(result, attribute)

		// Go to the next attribute.
		offset += int64(attribute.Length())
	}

	return result
}

// Open the MFT entry specified by a path name. Walks all directory
// indexes in the path to find the right MFT entry.
func (self *MFT_ENTRY) Open(ntfs *NTFSContext, filename string) (*MFT_ENTRY, error) {
	filename = strings.Replace(filename, "\\", "/", -1)
	components := strings.Split(path.Clean(filename), "/")

	get_path_in_dir := func(component string, dir *MFT_ENTRY) (
		*MFT_ENTRY, error) {

		// NTFS is usually case insensitive.
		component = strings.ToLower(component)

		for _, idx_record := range dir.Dir(ntfs) {
			item_name := strings.ToLower(idx_record.File().Name())

			if item_name == component {
				return ntfs.GetMFT(int64(
					idx_record.MftReference()))
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

func indent(input string) string {
	var indented []string
	for _, line := range strings.Split(input, "\n") {
		indented = append(indented, "  "+line)
	}

	return strings.Join(indented, "\n")
}

func (self *MFT_ENTRY) Display(ntfs *NTFSContext) string {
	result := []string{self.DebugString()}

	result = append(result, "Attribute:")
	for _, attr := range self.EnumerateAttributes(ntfs) {
		result = append(result, attr.PrintStats(ntfs))
	}

	return fmt.Sprintf("[MFT_ENTRY] @ %#0x\n", self.Offset) +
		strings.Join(result, "\n")
}

// Extract the $STANDARD_INFORMATION attribute from the MFT.
func (self *MFT_ENTRY) StandardInformation(ntfs *NTFSContext) (
	*STANDARD_INFORMATION, error) {
	for _, attr := range self.EnumerateAttributes(ntfs) {
		if attr.Type().Name == "$STANDARD_INFORMATION" {
			return self.Profile.STANDARD_INFORMATION(
				attr.Data(ntfs), 0), nil
		}
	}

	return nil, errors.New("$STANDARD_INFORMATION not found!")
}

// Extract the $FILE_NAME attribute from the MFT.
func (self *MFT_ENTRY) FileName(ntfs *NTFSContext) []*FILE_NAME {
	result := []*FILE_NAME{}

	for _, attr := range self.EnumerateAttributes(ntfs) {
		if attr.Type().Name == "$FILE_NAME" {
			res := self.Profile.FILE_NAME(attr.Data(ntfs), 0)
			result = append(result, res)
		}
	}

	return result
}

// Retrieve the content of the attribute stream specified by type and
// id. If id is 0 return the first attribute of this type.
func (self *MFT_ENTRY) GetAttribute(ntfs *NTFSContext,
	attr_type, id int64) (*NTFS_ATTRIBUTE, error) {
	for _, attr := range self.EnumerateAttributes(ntfs) {
		if attr.Type().Value == uint64(attr_type) {
			if id <= 0 || int64(attr.Attribute_id()) == id {
				return attr, nil
			}
		}
	}

	return nil, errors.New("Attribute not found!")
}

func (self *MFT_ENTRY) IsDir(ntfs *NTFSContext) bool {
	for _, attr := range self.EnumerateAttributes(ntfs) {
		switch attr.Type().Name {
		case "$INDEX_ROOT", "$INDEX_ALLOCATION":
			return true
		}
	}
	return false
}

func (self *MFT_ENTRY) Dir(ntfs *NTFSContext) []*INDEX_RECORD_ENTRY {
	result := []*INDEX_RECORD_ENTRY{}

	for _, node := range self.DirNodes(ntfs) {
		result = append(result, node.GetRecords(ntfs)...)
	}
	return result
}

func (self *MFT_ENTRY) DirNodes(ntfs *NTFSContext) []*INDEX_NODE_HEADER {
	result := []*INDEX_NODE_HEADER{}

	for _, attr := range self.EnumerateAttributes(ntfs) {
		switch attr.Type().Name {
		case "$INDEX_ROOT":
			index_root := self.Profile.INDEX_ROOT(
				attr.Data(ntfs), 0)
			result = append(result, index_root.Node())

		case "$INDEX_ALLOCATION":
			attr_reader := attr.Data(ntfs)
			for i := int64(0); i < int64(attr.DataSize()); i += 0x1000 {
				index_root, err := DecodeSTANDARD_INDEX_HEADER(
					ntfs, attr_reader, i, 0x1000)
				if err == nil {
					result = append(result, index_root.Node())
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
	// Very simple for now but faster for small number of runs.
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
