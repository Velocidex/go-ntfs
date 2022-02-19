package parser

import (
	"errors"
	"fmt"
	"path"
)

func get_longest_name(file_names []*FILE_NAME) string {
	result := ""
	for _, fn := range file_names {
		name := fn.Name()
		if len(result) < len(name) {
			result = name
		}
	}

	return result
}

// Traverse the mft entry and attempt to find its owner until the
// root. We return the full path of the MFT entry.
func GetFullPath(ntfs *NTFSContext, mft_entry *MFT_ENTRY) (string, error) {
	seen := []uint64{}
	return getFullPath(ntfs, mft_entry, seen)
}

func getFullPath(ntfs *NTFSContext, mft_entry *MFT_ENTRY,
	seen []uint64) (string, error) {

	if len(seen) > 10 {
		return "/", errors.New("Directory too deep")
	}

	// If the path is already cached, return it.
	if mft_entry.full_path != nil {
		return *mft_entry.full_path, nil
	}

	id := mft_entry.Record_number()
	if id == 5 {
		return "/", nil
	}

	full_path_any, pres := ntfs.full_path_lru.Get(int(id))
	if pres {
		return full_path_any.(string), nil
	}

	file_names := mft_entry.FileName(ntfs)
	if len(file_names) == 0 {
		return "/", fmt.Errorf("Entry %v has no filename", id)
	}
	parent_id := file_names[0].MftReference()
	// Check if the parent id is already seen
	for _, s := range seen {
		if s == parent_id {
			// Detected a loop
			return "/", nil
		}
	}
	seen = append(seen, parent_id)

	// Get the parent entry
	parent_mft_entry, err := ntfs.GetMFT(int64(parent_id))
	if err != nil {
		return "/", fmt.Errorf("Can not get Parent MFT %v", id)
	}

	parent_path, err := getFullPath(ntfs, parent_mft_entry, seen)
	if err != nil {
		return "/", err
	}

	longest_name := get_longest_name(file_names)
	full_path := path.Join(parent_path, longest_name)

	// Cache for next time.
	mft_entry.full_path = &full_path
	if mft_entry.IsDir(ntfs) {
		ntfs.full_path_lru.Add(int(id), full_path)
	}
	return full_path, nil
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
