package ntfs

import (
	"testing"

	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/go-ntfs/parser"
)

func TestDecompression(t *testing.T) {
	// One 6-byte chunk: header(0x8003) | tag=0x02 | 'A' | back-ref=0x0FFF.
	//   tag bit0 = 0 -> literal 'A'  (out len 1)
	//   tag bit1 = 1 -> back-reference; displacement=0,
	//     symbol_offset=(ptr>>12)+1=1, symbol_length=(ptr&0xFFF)+2=4097,
	//     copies 4098 bytes -> 4099 output bytes per 6 input bytes.
	block := []byte{0x03, 0x80, 0x02, 0x41, 0xFF, 0x0F}

	const nBlocks = 10000 // 60 000 input bytes
	in := make([]byte, 0, len(block)*nBlocks)
	for i := 0; i < nBlocks; i++ {
		in = append(in, block...)
	}

	// Should refuse to decompress file with uncompressed size too
	// large (100mb)
	_, err := parser.LZNT1Decompress(in)
	assert.Error(t, err)
}

func TestOOBRead(t *testing.T) {
	// Literal path: reads in[i] one byte past len(in).
	_, err := parser.LZNT1Decompress([]byte{0x01, 0x80, 0x00})
	assert.Error(t, err)

	// Back-reference path: Uint16(in[i:]) with <2 bytes remaining.
	_, err = parser.LZNT1Decompress([]byte{0x01, 0x80, 0x01})
	assert.Error(t, err)
}
