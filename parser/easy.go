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
	MFTId    string
	Mtime    time.Time
	Atime    time.Time
	Ctime    time.Time
	Name     string
	NameType string
	IsDir    bool
	Size     int64
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
	result := []*FileInfo{}
	for _, filename := range node_mft.FileName(ntfs) {
		for _, attr := range node_mft.EnumerateAttributes(ntfs) {
			attr_id := attr.Attribute_id()
			attr_type := attr.Type()

			// Only show MFT entries with either
			// $DATA or $INDEX_ROOT (files or
			// directies)
			is_dir := false
			switch attr.Type().Name {
			case "$DATA":
				is_dir = false
			case "$INDEX_ROOT":
				is_dir = true
			default:
				continue
			}

			inode := fmt.Sprintf(
				"%d-%d-%d", node_mft.Record_number(),
				attr_type.Value, attr_id)

			// Only show the first VCN run of
			// non-resident $DATA attributes.
			if !attr.IsResident() &&
				attr.Runlist_vcn_start() != 0 {
				continue
			}

			ads := ""
			name := attr.Name()
			switch name {
			case "$I30", "":
				ads = ""
			default:
				ads = ":" + name
			}

			result = append(result, &FileInfo{
				MFTId:    inode,
				Mtime:    filename.File_modified().Time,
				Atime:    filename.File_accessed().Time,
				Ctime:    filename.Mft_modified().Time,
				Name:     filename.Name() + ads,
				NameType: filename.name_type().Name,
				IsDir:    is_dir,
				Size:     attr.DataSize(),
			})
		}
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
