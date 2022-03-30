package parser

import (
	"errors"
	"fmt"
	"path"
)

func get_display_name(file_names []*FILE_NAME) string {
	short_name := ""
	for _, fn := range file_names {
		name := fn.Name()
		name_type := fn.NameType().Name
		switch name_type {
		case "Win32", "DOS+Win32", "POSIX":
			return name
		default:
			short_name = name
		}
	}

	return short_name
}

// Traverse the mft entry and attempt to find its owner until the
// root. We return the full path of the MFT entry.
func GetFullPath(ntfs *NTFSContext, mft_entry *MFT_ENTRY) (string, error) {
	components, err := GetComponents(ntfs, mft_entry)
	return "/" + path.Join(components...), err
}

// Traverse the mft entry and attempt to find its owner until the
// root. We return the full path of the MFT entry.
func GetComponents(ntfs *NTFSContext, mft_entry *MFT_ENTRY) ([]string, error) {
	seen := []uint64{}
	return getComponents(ntfs, mft_entry, seen)
}

func getComponents(ntfs *NTFSContext, mft_entry *MFT_ENTRY,
	seen []uint64) ([]string, error) {

	if len(seen) > 10 {
		return nil, errors.New("Directory too deep")
	}

	// If the path is already cached, return it.
	if len(mft_entry.components) > 0 {
		return mft_entry.components, nil
	}

	id := mft_entry.Record_number()
	if id == 5 {
		return nil, nil
	}

	components_any, pres := ntfs.full_path_lru.Get(int(id))
	if pres {
		return components_any.([]string), nil
	}

	file_names := mft_entry.FileName(ntfs)
	if len(file_names) == 0 {
		return nil, fmt.Errorf("Entry %v has no filename", id)
	}
	display_name := get_display_name(file_names)

	parent_id := file_names[0].MftReference()
	// Check if the parent id is already seen
	for _, s := range seen {
		if s == parent_id {
			// Detected a loop
			return []string{display_name}, nil
		}
	}
	seen = append(seen, parent_id)

	// Get the parent entry - either from cache or reparse it.
	parent_mft_entry, err := ntfs.GetMFT(int64(parent_id))
	if err != nil {
		return []string{display_name}, fmt.Errorf("Can not get Parent MFT %v", id)
	}

	parent_path_components, err := getComponents(ntfs, parent_mft_entry, seen)
	if err != nil {
		return []string{display_name}, err
	}

	new_components := appendComponents(parent_path_components,
		display_name)

	// Cache the mft entry for next time.
	mft_entry.components = new_components
	if mft_entry.IsDir(ntfs) {
		ntfs.full_path_lru.Add(int(id), new_components)
	}
	return new_components, nil
}

func CapUint64(v uint64, max uint64) uint64 {
	if v > max {
		return max
	}
	return v
}

func CapUint32(v uint32, max uint32) uint32 {
	if v > max {
		return max
	}
	return v
}

func CapUint16(v uint16, max uint16) uint16 {
	if v > max {
		return max
	}
	return v
}

func CapInt64(v int64, max int64) int64 {
	if v > max {
		return max
	}
	return v
}

func CapInt32(v int32, max int32) int32 {
	if v > max {
		return max
	}
	return v
}

func setADS(components []string, name string) []string {
	result := make([]string, 0, len(components))
	for i, c := range components {
		if i < len(components)-1 {
			result = append(result, c)
		} else {
			result = append(result, c+":"+name)
		}
	}
	return result
}

func appendComponents(components []string, name string) []string {
	result := make([]string, 0, len(components)+1)
	for _, c := range components {
		result = append(result, c)
	}

	result = append(result, name)
	return result
}
