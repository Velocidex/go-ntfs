// Implement some easy APIs.
package parser

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type FileInfo struct {
	MFTId      string
	Mtime      time.Time
	Atime      time.Time
	Ctime      time.Time
	Name       string
	NameType   string
	ExtraNames []string
	IsDir      bool
	Size       int64
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
		return components[0], 128, -1, nil
	case 2:
		return components[0], components[1], -1, nil
	case 3:
		return components[0], components[1], components[2], nil
	default:
		return 0, 0, 0, errors.New("Incorrect format for MFTId: e.g. 5-144-1")
	}
}

func GetDataForPath(ntfs *NTFSContext, path string) (io.ReaderAt, error) {
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
		if attr.Type().Name == "$DATA" &&
			attr.Name() == parts[1] {
			return attr.Data(ntfs), nil
		}
	}
	return nil, errors.New("File not found.")
}

func Stat(ntfs *NTFSContext, node_mft *MFT_ENTRY) []*FileInfo {
	var si *STANDARD_INFORMATION
	var other_file_names []*FILE_NAME
	var data_attributes []*NTFS_ATTRIBUTE
	var win32_name *FILE_NAME
	var index_attribute *NTFS_ATTRIBUTE
	var is_dir bool

	// Walk all the attributes collecting the imporant things.
	for _, attr := range node_mft.EnumerateAttributes(ntfs) {
		switch attr.Type().Name {
		case "$STANDARD_INFORMATION":
			si = ntfs.Profile.STANDARD_INFORMATION(attr.Data(ntfs), 0)

		case "$FILE_NAME":
			// Separate the filenames into LFN and other file names.
			file_name := ntfs.Profile.FILE_NAME(attr.Data(ntfs), 0)
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
