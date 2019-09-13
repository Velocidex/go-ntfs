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
	result := []string{}
	seen := make(map[uint32]bool)
	var err error

	for {
		id := mft_entry.Record_number()
		seen[id] = true

		file_names := mft_entry.FileName(ntfs)
		if len(file_names) == 0 {
			return path.Join(result...), errors.New(fmt.Sprintf(
				"Entry %v has no filename", id))
		}
		result = append([]string{get_longest_name(file_names)}, result...)

		mft_entry, err = ntfs.GetMFT(int64(file_names[0].MftReference()))
		if err != nil {
			return path.Join(result...), errors.New(fmt.Sprintf(
				"Entry %v has invalid parent", id))
		}
		_, pres := seen[mft_entry.Record_number()]
		if pres || len(result) > 20 {
			break
		}
	}
	return path.Join(result...), nil
}
