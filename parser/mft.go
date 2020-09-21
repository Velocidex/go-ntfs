package parser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
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

// See https://github.com/CCXLabs/CCXDigger/issues/13

// It is possible that an attribute list is pointing to an mft entry
// which also contains an attribute list. The second attribute list
// may also point to another entry inside the first MFT entry. This
// causes an infinite loop.

// Previous versions of the code erroneously called
// EnumerateAttributes to resolve a foreign attribute reference but
// this is not strictly correct because a foreign reference is never
// indirect and so never should traverse ATTRIBUTE_LISTs recursively
// anyway.

// The GetDirectAttribute() function looks for an exact attribute and
// type inside an MFT entry without following any attribute
// lists. This breaks the recursion and is a more correct approach.

// Search the MFT entry for a contained attribute - does not expand
// ATTRIBUTE_LISTs. This version is suitable to be called from within
// an ATTRIBUTE_LIST expansion.
func (self *MFT_ENTRY) GetDirectAttribute(
	ntfs *NTFSContext, attr_type uint64, attr_id uint16) (*NTFS_ATTRIBUTE, error) {
	offset := int64(self.Attribute_offset())

	for {
		// Instantiate the attribute over the fixed up address space.
		attribute := self.Profile.NTFS_ATTRIBUTE(self.Reader, offset)

		// Reached the end of the MFT entry.
		mft_size := int64(self.Mft_entry_size())
		attribute_size := int64(attribute.Length())
		if attribute_size == 0 ||
			attribute_size+offset > mft_size {
			break
		}

		if attribute.Type().Value == attr_type &&
			attribute.Attribute_id() == attr_id {
			return attribute, nil
		}

		// Go to the next attribute.
		offset += int64(attribute.Length())
	}
	return nil, errors.New("No attribute found.")
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

type MFTHighlight struct {
	EntryNumber          int64
	InUse                bool
	ParentEntryNumber    uint64
	FullPath             string
	FileName             string
	FileSize             int64
	ReferenceCount       int64
	IsDir                bool
	Created0x10          time.Time
	Created0x30          time.Time
	LastModified0x10     time.Time
	LastModified0x30     time.Time
	LastRecordChange0x10 time.Time
	LastRecordChange0x30 time.Time
	LastAccess0x10       time.Time
	LastAccess0x30       time.Time
}

func ParseMFTFile(
	ctx context.Context,
	reader io.ReaderAt,
	size int64,
	cluster_size int64,
	record_size int64) chan *MFTHighlight {
	output := make(chan *MFTHighlight)

	go func() {
		defer close(output)

		cache, _ := lru.New(1000)
		ntfs := &NTFSContext{
			DiskReader:  &NullReader{},
			Profile:     NewNTFSProfile(),
			ClusterSize: cluster_size,
			RecordSize:  record_size,
		}

		ntfs.RootMFT = ntfs.Profile.MFT_ENTRY(reader, 0)

		for i := int64(0); i < size; i += record_size {
			mft_entry, err := getMFT_ENTRY(ntfs, reader, i)
			if err != nil {
				continue
			}

			var file_names []*FILE_NAME
			var si *STANDARD_INFORMATION
			var size int64

			for _, attr := range mft_entry.EnumerateAttributes(ntfs) {
				attr_type := attr.Type()
				switch attr_type.Name {
				case "$DATA":
					if size == 0 {
						size = attr.DataSize()
					}
				case "$FILE_NAME":
					res := ntfs.Profile.FILE_NAME(
						attr.Data(ntfs), 0)
					file_names = append(file_names, res)

				case "$STANDARD_INFORMATION":
					si = ntfs.Profile.STANDARD_INFORMATION(
						attr.Data(ntfs), 0)
				}
			}
			if len(file_names) == 0 {
				continue
			}
			if si == nil {
				continue
			}

			full_path, err := getFullPathWithCache(ntfs, mft_entry,
				file_names, cache)
			if err != nil {
				continue
			}

			mft_id := mft_entry.Record_number()

			output <- &MFTHighlight{
				EntryNumber:          int64(mft_id),
				InUse:                mft_entry.Flags().IsSet("ALLOCATED"),
				ParentEntryNumber:    file_names[0].MftReference(),
				FullPath:             full_path,
				FileName:             get_longest_name(file_names),
				FileSize:             size,
				ReferenceCount:       int64(mft_entry.Link_count()),
				IsDir:                mft_entry.Flags().IsSet("DIRECTORY"),
				Created0x10:          si.Create_time().Time,
				Created0x30:          file_names[0].Created().Time,
				LastModified0x10:     si.File_altered_time().Time,
				LastModified0x30:     file_names[0].File_modified().Time,
				LastRecordChange0x10: si.Mft_altered_time().Time,
				LastRecordChange0x30: file_names[0].Mft_modified().Time,
				LastAccess0x10:       si.File_accessed_time().Time,
				LastAccess0x30:       file_names[0].File_accessed().Time,
			}

			// Check for cancellations.
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
	}()

	return output
}

func getFullPathWithCache(ntfs *NTFSContext,
	mft_entry *MFT_ENTRY,
	file_names []*FILE_NAME,
	cache *lru.Cache) (string, error) {
	var full_path string

	id := mft_entry.Record_number()

	my_name := get_longest_name(file_names)

	// Get the parents full path from cache if possible
	parent_mft_id := int64(file_names[0].MftReference())
	full_path_any, pres := cache.Get(parent_mft_id)
	if !pres {
		parent_mft_entry, err := ntfs.GetMFT(parent_mft_id)
		if err != nil {
			return "", errors.New(fmt.Sprintf(
				"Entry %v has invalid parent", id))
		}

		full_path, err = GetFullPath(ntfs, parent_mft_entry)
		if err != nil {
			full_path = "<unknown>"
		}
		cache.Add(parent_mft_id, full_path)
	} else {
		full_path = full_path_any.(string)
	}

	return path.Join(full_path, my_name), nil
}

func getMFT_ENTRY(ctx *NTFSContext,
	reader io.ReaderAt, id int64) (*MFT_ENTRY, error) {
	disk_mft := ctx.Profile.MFT_ENTRY(reader, id)

	// Uninitialized entries will have invalid fixups.
	fixed_up_reader, err := FixUpDiskMFTEntry(disk_mft)
	if err != nil {
		return nil, err
	}

	return ctx.Profile.MFT_ENTRY(fixed_up_reader, 0), nil
}
