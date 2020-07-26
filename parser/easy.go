// Implement some easy APIs.
package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type FileInfo struct {
	MFTId         string    `json:"MFTId,omitempty"`
	Mtime         time.Time `json:"Mtime,omitempty"`
	Atime         time.Time `json:"Atime,omitempty"`
	Ctime         time.Time `json:"Ctime,omitempty"`
	Btime         time.Time `json:"Btime,omitempty"` // Birth time.
	Name          string    `json:"Name,omitempty"`
	NameType      string    `json:"NameType,omitempty"`
	ExtraNames    []string  `json:"ExtraNames,omitempty"`
	IsDir         bool      `json:"IsDir,omitempty"`
	Size          int64
	AllocatedSize int64

	// Is it in I30 slack?
	IsSlack     bool  `json:"IsSlack,omitempty"`
	SlackOffset int64 `json:"SlackOffset,omitempty"`
}

func GetNTFSContext(image io.ReaderAt, offset int64) (*NTFSContext, error) {
	ntfs := &NTFSContext{
		DiskReader: image,
		Profile:    NewNTFSProfile(),
	}

	// NTFS Parsing starts with the boot record.
	ntfs.Boot = &NTFS_BOOT_SECTOR{Reader: image,
		Profile: ntfs.Profile, Offset: offset}

	err := ntfs.Boot.IsValid()
	if err != nil {
		return nil, err
	}

	ntfs.ClusterSize = ntfs.Boot.ClusterSize()

	mft_reader, err := BootstrapMFT(ntfs)
	if err != nil {
		return nil, err
	}

	ntfs.RootMFT = ntfs.Profile.MFT_ENTRY(mft_reader, 5)

	return ntfs, nil
}

func ParseMFTId(mft_id string) (mft_idx int64, attr int64, id int64, err error) {
	components := []int64{}
	components_str := strings.Split(mft_id, "-")
	for _, component_str := range components_str {
		x, err := strconv.Atoi(component_str)
		if err != nil {
			return 0, 0, 0, errors.New("Incorrect format for MFTId: e.g. 5-144-1")
		}

		components = append(components, int64(x))
	}

	switch len(components) {
	case 1:
		return components[0], 128, 0, nil
	case 2:
		return components[0], components[1], 0, nil
	case 3:
		return components[0], components[1], components[2], nil
	default:
		return 0, 0, 0, errors.New("Incorrect format for MFTId: e.g. 5-144-1")
	}
}

func GetDataForPath(ntfs *NTFSContext, path string) (RangeReaderAt, error) {
	// Check for ADS in the path.
	parts := strings.Split(path, ":")
	switch len(parts) {
	case 2:
		break
	case 1:
		parts = append(parts, "")
	default:
		return nil, errors.New("Path may not contain more than one ':'")
	}

	root, err := ntfs.GetMFT(5)
	if err != nil {
		return nil, err
	}

	mft_entry, err := root.Open(ntfs, parts[0])
	if err != nil {
		return nil, err
	}

	for _, attr := range mft_entry.EnumerateAttributes(ntfs) {
		if attr.Type().Value == 128 &&
			attr.Name() == parts[1] {
			return OpenStream(ntfs, mft_entry,
				128, attr.Attribute_id())
		}
	}

	return nil, errors.New("File not found.")
}

func RangeSize(rng RangeReaderAt) int64 {
	runs := rng.Ranges()
	if len(runs) == 0 {
		return 0
	}

	last_run := runs[len(runs)-1]
	return last_run.Offset + last_run.Length
}

func Stat(ntfs *NTFSContext, node_mft *MFT_ENTRY) []*FileInfo {
	var si *STANDARD_INFORMATION
	var other_file_names []*FILE_NAME
	var data_attributes []*NTFS_ATTRIBUTE
	var win32_name *FILE_NAME
	var index_attribute *NTFS_ATTRIBUTE
	var is_dir bool

	var birth_time time.Time

	// Walk all the attributes collecting the imporant things.
	for _, attr := range node_mft.EnumerateAttributes(ntfs) {
		switch attr.Type().Name {
		case "$STANDARD_INFORMATION":
			si = ntfs.Profile.STANDARD_INFORMATION(attr.Data(ntfs), 0)

		case "$FILE_NAME":
			// Separate the filenames into LFN and other file names.
			file_name := ntfs.Profile.FILE_NAME(attr.Data(ntfs), 0)

			// The birth of an MFT is determined by the
			// $FILE_NAME streams File_modified attribute
			// since it can not modified using normal
			// APIs.
			birth_time = file_name.File_modified().Time

			switch file_name.NameType().Name {
			case "POSIX", "Win32", "DOS+Win32":
				win32_name = file_name
			default:
				other_file_names = append(
					other_file_names, file_name)
			}

		case "$DATA":
			// Only show the first VCN run of
			// non-resident $DATA attributes.
			if !attr.IsResident() &&
				attr.Runlist_vcn_start() != 0 {
				continue
			}

			data_attributes = append(data_attributes, attr)

		case "$INDEX_ROOT", "$INDEX_ALLOCATION":
			is_dir = true
			index_attribute = attr
		}
	}

	// We need the si for the timestamps.
	if si == nil || win32_name == nil {
		return nil
	}

	// Now generate multiple file info for streams we want to be
	// distinct.
	result := []*FileInfo{}

	add_extra_names := func(info *FileInfo, ads string) {
		for _, name := range other_file_names {
			extra_name := name.Name()
			info.ExtraNames = append(info.ExtraNames, extra_name+ads)

			if !strings.Contains(extra_name, "~") {
				// Make a copy
				info_copy := *info
				info_copy.Name = extra_name + ads
				info_copy.ExtraNames = []string{win32_name.Name() + ads}
				result = append(result, &info_copy)
			}
		}
	}

	if index_attribute != nil {
		inode := fmt.Sprintf(
			"%d-%d-%d", node_mft.Record_number(),
			index_attribute.Type().Value,
			index_attribute.Attribute_id())

		info := &FileInfo{
			MFTId:    inode,
			Mtime:    si.File_altered_time().Time,
			Atime:    si.File_accessed_time().Time,
			Ctime:    si.Mft_altered_time().Time,
			Btime:    birth_time,
			Name:     win32_name.Name(),
			NameType: win32_name.NameType().Name,
			IsDir:    is_dir,
		}

		add_extra_names(info, "")
		result = append(result, info)
	}

	for _, attr := range data_attributes {
		ads := ""
		name := attr.Name()
		switch name {
		case "$I30", "":
			ads = ""
		default:
			ads = ":" + name
		}

		inode := fmt.Sprintf(
			"%d-%d-%d", node_mft.Record_number(),
			attr.Type().Value,
			attr.Attribute_id())

		info := &FileInfo{
			MFTId:    inode,
			Mtime:    si.File_altered_time().Time,
			Atime:    si.File_accessed_time().Time,
			Ctime:    si.Mft_altered_time().Time,
			Btime:    birth_time,
			Name:     win32_name.Name() + ads,
			NameType: win32_name.NameType().Name,
			IsDir:    is_dir,
			Size:     attr.DataSize(),
		}

		add_extra_names(info, ads)

		result = append(result, info)
	}

	return result
}

func ListDir(ntfs *NTFSContext, root *MFT_ENTRY) []*FileInfo {
	// The index itself stores pointers to the FILE_NAME entry for
	// each MFT. Therefore there are usually 2 references to the
	// same MFT entry. We de-duplicate these references because we
	// list each MFT entirely separately.
	seen := make(map[int64]bool)
	result := []*FileInfo{}

	for _, node := range root.Dir(ntfs) {
		node_mft_id := int64(node.MftReference())
		_, pres := seen[node_mft_id]
		if pres {
			continue
		}
		seen[node_mft_id] = true

		node_mft, err := ntfs.GetMFT(node_mft_id)
		if err != nil {
			continue
		}
		result = append(result, Stat(ntfs, node_mft)...)
	}
	return result
}

// Get all VCNs having the same type and ID
func getAllVCNs(ntfs *NTFSContext,
	mft_entry *MFT_ENTRY, attr_type uint64, attr_id uint16) []*NTFS_ATTRIBUTE {
	result := []*NTFS_ATTRIBUTE{}
	for _, attr := range mft_entry.EnumerateAttributes(ntfs) {
		if attr.Type().Value == attr_type &&
			attr.Attribute_id() == attr_id {
			result = append(result, attr)
		}
	}
	return result
}

func (self *NTFS_ATTRIBUTE) getVCNReader(ntfs *NTFSContext,
	start, length, compression_unit_size int64) *MappedReader {

	if self.Resident().Name == "RESIDENT" {
		buf := make([]byte, self.Content_size())
		n, _ := self.Reader.ReadAt(
			buf,
			self.Offset+int64(self.Content_offset()))
		buf = buf[:n]

		return &MappedReader{
			FileOffset:  0,
			Length:      int64(n),
			ClusterSize: 1,
			Reader:      bytes.NewReader(buf),
		}

		// Stream is compressed
	} else if self.Flags().IsSet("COMPRESSED") {
		return &MappedReader{
			ClusterSize: 1,
			FileOffset:  start,
			Length:      length,
			Reader: NewCompressedRangeReader(self.RunList(),
				ntfs.ClusterSize,
				ntfs.DiskReader,
				compression_unit_size),
		}
	}

	// If the attribute is not fully initialized, trim the mapping
	// to the initialized range. For example, the attribute might
	// contain 32 clusters, but only 16 clusters are initialized.
	initialized_size := int64(self.Initialized_size())
	if length > initialized_size {
		length = initialized_size
	}

	return &MappedReader{
		FileOffset:  start,
		Length:      length,
		ClusterSize: 1,
		Reader: NewRangeReader(
			self.RunList(), ntfs.DiskReader,
			ntfs.ClusterSize, compression_unit_size),
	}
}

// Open the full stream. Note - In NTFS a stream can be composed of
// multiple VCN attributes: All VCN substreams have the same attribute
// type and id but different start and end VCNs. This function finds
// all related attributes and wraps them in a RangeReader to appear as
// a single stream. This function is what you need when you want to
// read the full file.
func OpenStream(ntfs *NTFSContext,
	mft_entry *MFT_ENTRY, attr_type uint64, attr_id uint16) (RangeReaderAt, error) {

	attr_id_found := false

	result := &RangeReader{}

	size := int64(0)
	compression_unit_size := int64(0)

	for _, attr := range mft_entry.EnumerateAttributes(ntfs) {
		if attr.Type().Value != attr_type {
			continue
		}

		// An attr id of 0 means take the first ID of the required type.
		if attr_id == 0 && !attr_id_found {
			attr_id = attr.Attribute_id()
			attr_id_found = true
		}

		if attr.Attribute_id() != attr_id {
			continue
		}

		start := int64(attr.Runlist_vcn_start()) * ntfs.ClusterSize
		end := int64(attr.Runlist_vcn_end()+1) * ntfs.ClusterSize

		// Actual_size is only set on the first stream.
		if size == 0 {
			size = int64(attr.Actual_size())
		}

		// Cap the length of the stream at the file's
		// actual size (the size may not be cluster
		// aligned but Runlist_vcn_end() always refers
		// to the last cluster number).
		if end > size {
			end = size
		}

		// Compression_unit_size is only set on the first stream.
		if compression_unit_size == 0 {
			compression_unit_size = int64(
				1 << uint64(attr.Compression_unit_size()))
		}

		length := end - start

		reader := attr.getVCNReader(ntfs, start, length,
			compression_unit_size)
		result.runs = append(result.runs, reader)

		// If the returns mapping does not cover the entire
		// range we need, add a pad mapping to the end so we
		// do not have gaps.
		if attr.Resident().Name != "RESIDENT" && reader.Length < end-start {
			pad := &MappedReader{
				ClusterSize: 1,
				FileOffset:  reader.FileOffset + reader.Length,
				Length:      end - start - reader.Length,
				IsSparse:    true,
				Reader:      &NullReader{},
			}
			result.runs = append(result.runs, pad)
		}
	}

	return result, nil
}
