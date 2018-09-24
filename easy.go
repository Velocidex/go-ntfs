// Implement some easy APIs.
package ntfs

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

func GetRootMFTEntry(image io.ReaderAt) (*MFT_ENTRY, error) {
	profile, err := GetProfile()
	if err != nil {
		return nil, err
	}

	boot, err := NewBootRecord(profile, image, 0)
	if err != nil {
		return nil, err
	}

	mft, err := boot.MFT()
	if err != nil {
		return nil, err
	}

	return mft.MFTEntry(5)
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

func GetDataForPath(path string, root *MFT_ENTRY) (io.ReaderAt, error) {
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

	mft_entry, err := root.Open(parts[0])
	if err != nil {
		return nil, err
	}

	for _, attr := range mft_entry.Attributes() {
		if attr.Get("type").AsInteger() == 128 &&
			attr.Name() == parts[1] {
			return mft_entry.Data(
				128, attr.Get("attribute_id").AsInteger()), nil
		}
	}
	return nil, errors.New("File not found.")
}

func ListDir(root *MFT_ENTRY) []*FileInfo {
	// The index itself stores pointers to the FILE_NAME entry for
	// each MFT. Therefore there are usually 2 references to the
	// same MFT entry. We de-duplicate these references because we
	// list each MFT entirely separately.
	seen := make(map[int64]bool)
	result := []*FileInfo{}

	for _, node := range root.Dir() {
		node_mft_id := node.Get("mftReference").AsInteger()
		_, pres := seen[node_mft_id]
		if pres {
			continue
		}
		seen[node_mft_id] = true

		node_mft, err := root.MFTEntry(node_mft_id)
		if err != nil {
			continue
		}

		for _, filename := range node_mft.FileName() {
			for _, attr := range node_mft.Attributes() {
				attr_id := attr.Get("attribute_id").AsInteger()
				is_dir := false
				attr_type := attr.Get("type").AsInteger()
				switch attr_type {
				case 128: // $DATA attribute.
					is_dir = false
				case 144: // $INDEX_ROOT
					is_dir = true
				default:
					continue

				}

				inode := fmt.Sprintf(
					"%d-%d-%d", node_mft_id, attr_type, attr_id)

				// Only show the first VCN run
				// of non-resident $DATA
				// attributes.
				if !attr.IsResident() &&
					attr.Get("runlist_vcn_start").
						AsInteger() != 0 {
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
					MFTId: inode,
					Mtime: time.Unix(
						filename.Get("file_modified").
							AsInteger(), 0),
					Atime: time.Unix(
						filename.Get("file_accessed").
							AsInteger(), 0),
					Ctime: time.Unix(
						filename.Get("mft_modified").
							AsInteger(), 0),
					Name:     filename.Name() + ads,
					NameType: filename.Get("name_type").AsString(),
					IsDir:    is_dir,
					Size:     attr.Size(),
				})
			}
		}
	}
	return result
}
