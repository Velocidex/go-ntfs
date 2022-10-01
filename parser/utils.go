package parser

import (
	"errors"
	"fmt"
	"path"
)

var (
	TooDeepError       = errors.New("Dir too deep")
	NameNotFoundError  = errors.New("Entry has no filename")
	LoopError          = errors.New("Loop Detected")
	InvalidParentEntry = errors.New("Can not get Parent MFT")
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
	res, err := getComponents(ntfs, mft_entry, seen)
	return res, err
}

func getComponents(ntfs *NTFSContext, mft_entry *MFT_ENTRY,
	seen []uint64) ([]string, error) {

	// Allow the directory depth to be configured (default 20)
	if len(seen) > ntfs.MaxDirectoryDepth {
		return []string{"<Err>",
			"<Error-DirTooDeep>"}, TooDeepError
	}

	// If the path components are already cached, return a copy.
	mft_entry.mu.Lock()
	mft_entry_components := CopySlice(mft_entry.components)
	mft_entry.mu.Unlock()

	if len(mft_entry_components) > 0 {
		return mft_entry_components, nil
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
		return []string{"<Err>",
			fmt.Sprintf("<Unknown entry %v>", id)}, NameNotFoundError
	}
	display_name := get_display_name(file_names)

	parent_id := file_names[0].MftReference()
	// Check if the parent id is already seen
	for _, s := range seen {
		if s == parent_id {
			// Detected a loop
			return []string{"<Err>",
				fmt.Sprintf("<Loop detected %v>", parent_id),
				display_name}, LoopError
		}
	}
	seen = append(seen, parent_id)

	// Get the parent sequence number.
	parent_sequence_number := file_names[0].Seq_num()

	// Get the parent entry - either from cache or reparse it.
	parent_mft_entry, err := ntfs.GetMFT(int64(parent_id))
	if err != nil {
		return []string{"<Err>",
			fmt.Sprintf("<Error Parent %v (%v)>", parent_id, err),
			display_name}, InvalidParentEntry
	}

	if parent_sequence_number != parent_mft_entry.Sequence_value() {
		return []string{"<Err>",
			fmt.Sprintf("<Error Parent %v-%v need %v>", parent_id,
				parent_mft_entry.Sequence_value(), parent_sequence_number),
			display_name}, InvalidParentEntry
	}

	parent_path_components, err := getComponents(ntfs, parent_mft_entry, seen)
	if err != nil {
		// If we hit an error ensure not to cache the path.
		return append(parent_path_components, display_name), err
	}

	new_components := append(parent_path_components, display_name)

	// Cache the mft entry for next time.
	mft_entry.mu.Lock()
	mft_entry.components = new_components
	mft_entry.mu.Unlock()

	// Only cache directories because files usually do not contain
	// children so there is no benefit in caching
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

func CopySlice(in []string) []string {
	result := make([]string, len(in))
	copy(result, in)
	return result
}
