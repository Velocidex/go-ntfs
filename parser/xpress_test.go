package parser

import (
	"testing"
)

func TestXpressPlainLZ77(t *testing.T) {
	for _, v := range xpressVectors {
		if v.huff {
			continue
		}
		out, err := XpressDecompress(v.data)
		if err != nil {
			t.Errorf("%s: %v", v.name, err)
			continue
		}
		if string(out) != string(v.expected) {
			t.Errorf("%s: mismatched output (%d bytes)", v.name, len(out))
		}
	}
}

func TestXpressHuffman(t *testing.T) {
	for _, v := range xpressVectors {
		if !v.huff {
			continue
		}
		out, err := XpressHuffmanDecompress(v.data, len(v.expected))
		if err != nil {
			t.Errorf("%s: %v", v.name, err)
			continue
		}
		if string(out) != string(v.expected) {
			t.Errorf("%s: mismatched output (%d bytes)", v.name, len(out))
		}
	}
}

func TestXpressTruncated(t *testing.T) {
	// Plain LZ77: the MS-XCA worked example, cut mid-match — the
	// nibble/extra-length bytes are missing.
	in := []byte{
		0xff, 0xff, 0xff, 0x1f, // flag group: literal, literal, literal, match
		0x61, 0x62, 0x63, // "abc"
		0x17, 0x00, 0x0f, // match, then the last nibble byte runs off the end
	}
	_, err := XpressDecompress(in)
	if err == nil {
		t.Error("plain: expected error on truncated stream")
	}

	// Huffman: fewer than the mandatory 256 table bytes.
	var huffShort *xpressVector
	var huffSmall *xpressVector
	for i := range xpressVectors {
		if !xpressVectors[i].huff {
			continue
		}
		if huffShort == nil && len(xpressVectors[i].data) > 100 {
			huffShort = &xpressVectors[i]
		}
		out, err := XpressHuffmanDecompress(xpressVectors[i].data, len(xpressVectors[i].expected))
		if err == nil && len(out) > 22 && huffSmall == nil {
			// Declaring a smaller output than this stream produces
			// must be rejected, not silently truncated.
			if _, err := XpressHuffmanDecompress(xpressVectors[i].data, 22); err != nil {
				huffSmall = &xpressVectors[i]
			}
		}
	}
	if huffShort == nil {
		t.Fatal("no huffman vectors")
	}
	_, err = XpressHuffmanDecompress(huffShort.data[:100], 360)
	if err == nil {
		t.Error("huffman: expected error on short table")
	}

	// Huffman: declare a smaller output than the stream produces.
	if huffSmall == nil {
		t.Fatal("no huffman vector rejects a too-small output size")
	}
	_, err = XpressHuffmanDecompress(huffSmall.data, 22)
	if err == nil {
		t.Error("huffman: expected error when output size is too small")
	}
}

func TestXpressExpansionRatio(t *testing.T) {
	// A match with a 4GB raw length via the LE32 extension must be
	// rejected by the decompressed-size cap.
	in := []byte{
		0x40, 0x00, 0x00, 0x00, // flag group: literal, then match
		0x41,       // literal 'A'
		0x07, 0x00, // match: offset 1, length nibble 7
		0xFF,       // nibble 15: raw length byte
		0x00, 0x00, // -> 255: LE16
		0xFF, 0xFF, 0xFF, 0xFF, // -> 0: LE32 (4GB - 1)
	}
	_, err := XpressDecompress(in)
	if err == nil {
		t.Error("plain: expected compression ratio error")
	}
}

func TestXpressCorruptCode(t *testing.T) {
	// Huffman table of all-zero lengths is an incomplete prefix code.
	in := make([]byte, 260)
	_, err := XpressHuffmanDecompress(in, 1)
	if err == nil {
		t.Error("huffman: expected error on invalid code lengths")
	}
}

func TestXpressOOB(t *testing.T) {
	// Flags group claims a match but there are no bytes left.
	_, err := XpressDecompress([]byte{0x80, 0x00, 0x00, 0x00})
	if err == nil {
		t.Error("plain: expected error on dangling match flag")
	}

	// A match farther back than the output produced so far.
	_, err = XpressDecompress([]byte{0x40, 0x00, 0x00, 0x00, 0x41, 0x50, 0x00})
	if err == nil {
		t.Error("plain: expected error on out-of-range offset")
	}
}
