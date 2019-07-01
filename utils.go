package ntfs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path"
	"unicode/utf16"
)

func UTF16ToString(bytes []byte) string {
	order := binary.LittleEndian
	u16s := []uint16{}

	for i, j := 0, len(bytes); i < j; i += 2 {
		if len(bytes) < i+2 {
			break
		}
		u16s = append(u16s, order.Uint16(bytes[i:]))
	}

	return string(utf16.Decode(u16s))
}

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
func GetFullPath(mft_entry *MFT_ENTRY) (string, error) {
	result := []string{}
	seen := make(map[int64]bool)
	var err error

	for {
		id := mft_entry.Get("record_number").AsInteger()
		seen[id] = true

		file_names := mft_entry.FileName()
		if len(file_names) == 0 {
			return path.Join(result...), errors.New(fmt.Sprintf(
				"Entry %v has no filename", id))
		}
		result = append([]string{get_longest_name(file_names)}, result...)

		mft_entry, err = mft_entry.MFTEntry(
			file_names[0].Get("mftReference").AsInteger())
		if err != nil {
			return path.Join(result...), errors.New(fmt.Sprintf(
				"Entry %v has invalid parent", id))
		}
		_, pres := seen[mft_entry.Get("record_number").AsInteger()]
		if pres || len(result) > 20 {
			break
		}
	}
	return path.Join(result...), nil
}
