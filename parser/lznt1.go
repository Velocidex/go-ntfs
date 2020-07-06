/*
Decompression support for the LZNT1 compression algorithm.

Reference:
http://msdn.microsoft.com/en-us/library/jj665697.aspx
(2.5 LZNT1 Algorithm Details)

https://github.com/libyal/reviveit/
https://github.com/sleuthkit/sleuthkit/blob/develop/tsk/fs/ntfs.c
*/

package parser

import (
	"encoding/binary"
	"errors"
)

var (
	COMPRESSED_MASK = uint16(1 << 15)
	SIGNATURE_MASK  = uint16(3 << 12)
	SIZE_MASK       = uint16(1<<12) - 1
)

func get_displacement(offset int) byte {
	result := byte(0)
	for {
		if offset < 0x10 {
			return result
		}

		offset >>= 1
		result += 1
	}
}

func LZNT1Decompress(in []byte) ([]byte, error) {
	// Index into the in buffer
	i := 0
	out := []byte{}

	for {
		if len(in) < i+2 {
			break
		}
		uncompressed_chunk_offset := len(out)
		block_offset := i

		block_header := binary.LittleEndian.Uint16(in[i:])
		LZNT1Printf("Header %#x @ %#x %d\n", block_header, i, i)
		i += 2

		size := int(block_header & SIZE_MASK)
		block_end := block_offset + size + 3
		LZNT1Printf("%d Block Size: %d ends at %d\n", len(out), size+3, block_end)
		if size == 0 {
			break
		}

		if len(in) < i+size {
			return nil, errors.New("Block too small!")
		}

		if block_header&COMPRESSED_MASK != 0 {
			for i < block_end {
				header := in[i]
				LZNT1Printf("%d Tag %x\n", len(out), header)
				i++

				for mask_idx := uint8(0); mask_idx < 8 && i < block_end; mask_idx++ {
					mask := byte(1 << mask_idx)
					if mask&header != 0 {
						pointer := binary.LittleEndian.Uint16(in[i:])
						i += 2

						displacement := get_displacement(
							len(out) - uncompressed_chunk_offset - 1)
						symbol_offset := int(pointer>>(12-displacement)) + 1
						symbol_length := int(pointer&(0xFFF>>displacement)) + 2
						LZNT1Printf("Wrote %d @ %d/%d: Phrase %d %d %x\n",
							symbol_length, i, len(out),
							symbol_length, symbol_offset, pointer)
						start_offset := len(out) - symbol_offset
						for j := 0; j < symbol_length+1; j++ {
							idx := start_offset + j
							if idx < 0 || idx >= len(out) {
								return nil, errors.New("Decompression error - shift is too large")
							}
							out = append(out, out[idx])
						}
					} else {
						LZNT1Printf("%d: Symbol %#x (%d)\n", i, in[i], len(out))
						out = append(out, in[i])
						i++
					}
				}
			}
			// Block is not compressed.
		} else {
			out = append(out, in[i:i+size+1]...)
			i += size + 1
		}

	}

	return out, nil
}
