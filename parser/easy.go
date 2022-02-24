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
	FNBtime       time.Time `json:"FNBtime,omitempty"`
	FNMtime       time.Time `json:"FNBtime,omitempty"`
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
	ntfs := newNTFSContext(image, "GetNTFSContext")

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

	var fn_birth_time, fn_mtime time.Time

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
			fn_birth_time = file_name.Created().Time
			fn_mtime = file_name.Created().Time

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
			Btime:    si.Create_time().Time,
			FNBtime:  fn_birth_time,
			FNMtime:  fn_mtime,
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
			Btime:    si.Create_time().Time,
			FNBtime:  fn_birth_time,
			FNMtime:  fn_mtime,
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

// Gets a reader over this attribute's VCN. The actual_size and
// initialized_size are the file's complete sizes - these are normally
// only filled in for the first VCN attribute.
func (self *NTFS_ATTRIBUTE) getVCNReader(ntfs *NTFSContext,
	actual_size, initialized_size,
	compression_unit_size int64) ([]*MappedReader, int64) {

	// Handle resident attributes specifically.
	if self.Resident().Name == "RESIDENT" {
		buf := make([]byte, CapUint32(self.Content_size(), MAX_MFT_ENTRY_SIZE))
		n, _ := self.Reader.ReadAt(
			buf,
			self.Offset+int64(self.Content_offset()))
		buf = buf[:n]

		return []*MappedReader{
			&MappedReader{
				FileOffset:  0,
				Length:      int64(n),
				ClusterSize: 1,
				Reader:      bytes.NewReader(buf),
			}}, int64(n)
	}

	// Figure out how much of the file this attribute covers.
	start := int64(self.Runlist_vcn_start()) * ntfs.ClusterSize
	end := int64(self.Runlist_vcn_end()+1) * ntfs.ClusterSize

	// This attribute covers the range between the end vcn and the
	// start vcn
	length := end - start

	// Cap this run length at the file's actual_size in case the
	// attribute's VCN is over allocated past the end of the file.
	if length > actual_size {
		length = actual_size
	}

	if self.Flags().IsSet("COMPRESSED") {
		return []*MappedReader{&MappedReader{
			ClusterSize: 1,
			FileOffset:  start,
			Length:      length,
			Reader: NewCompressedRangeReader(self.RunList(),
				ntfs.ClusterSize,
				ntfs.DiskReader,
				compression_unit_size),
		}}, length
	}

	// Handle a non compressed VCN

	// If the attribute is not fully initialized, trim the mapping
	// to the initialized range and add a pad to the end to make
	// up the full length. For example, the attribute might
	// contain 32 clusters, but only 16 clusters are initialized
	// and 16 clusters are padding.
	if length > initialized_size {
		return []*MappedReader{
			&MappedReader{
				FileOffset:  start,
				Length:      initialized_size,
				ClusterSize: 1,
				Reader: NewRangeReader(
					self.RunList(), ntfs.DiskReader,
					ntfs.ClusterSize, compression_unit_size),
			},
			// Pad starts immediately after the last range
			&MappedReader{
				ClusterSize: 1,
				FileOffset:  start + initialized_size,
				Length:      length - initialized_size,
				IsSparse:    true,
				Reader:      &NullReader{},
			}}, length
	}

	return []*MappedReader{
		&MappedReader{
			FileOffset:  start,
			Length:      length,
			ClusterSize: 1,
			Reader: NewRangeReader(
				self.RunList(), ntfs.DiskReader,
				ntfs.ClusterSize, compression_unit_size),
		}}, length
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

	actual_size := int64(0)
	initialized_size := int64(0)
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

		// Actual_size is only set on the first stream.
		if actual_size == 0 {
			actual_size = int64(attr.Actual_size())
		}

		// Initialized_size is only set on the first stream
		if initialized_size == 0 {
			initialized_size = int64(attr.Initialized_size())
		}

		// Compression_unit_size is only set on the first stream.
		if compression_unit_size == 0 {
			compression_unit_size = int64(
				1 << uint64(attr.Compression_unit_size()))
		}

		reader, consumed_length := attr.getVCNReader(ntfs, actual_size, initialized_size,
			compression_unit_size)
		result.runs = append(result.runs, reader...)

		actual_size -= consumed_length
		initialized_size -= consumed_length
	}

	return result, nil
}
