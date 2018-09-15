package ntfs

import (
	"encoding/binary"
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
