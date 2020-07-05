package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input []Run
	out   []MappedReader
}

type readerTestCase struct {
	input []Run
	out   []Range
}

var (
	TestCases = []testCase{
		{input: []Run{
			{474540, 47},
			{0, 1},
			{48, 1213},
			{0, 3},
		}, out: []MappedReader{
			{0, 474540, 32, 0x400, 0, false, nil},
			{32, 474572, 16, 0x400, 15, false, nil},
			{48, 474588, 1200, 0x400, 0, false, nil},
			{1248, 475788, 16, 0x400, 13, false, nil},
		}},
		// A compressed run followed by a sparse run longer
		// than compression size.
		{input: []Run{
			{1940823, 2},
			{0, 30}, // This is really {0, 14}, {0, 16} merged together.
		}, out: []MappedReader{

			// A compressed run followed by sparse run.
			{0, 1940823, 16, 0x400, 2, false, nil},
			{16, 0, 16, 0x400, 0, false, nil},
		}},
	}

	ReaderTestCases = []readerTestCase{
		{input: []Run{
			{474540, 47},
			{0, 1},
			{48, 1213},
			{0, 3},
		}, out: []Range{
			{0, 32 * 0x400, false},
			{32 * 0x400, 16 * 0x400, false},
			{48 * 0x400, 1200 * 0x400, false},
			{1248 * 0x400, 16 * 0x400, false},
		}},

		// A compressed run followed by a sparse run longer
		// than compression size.
		{input: []Run{
			{1940823, 2},
			{0, 30}, // This is really {0, 14}, {0, 16} merged together.
		}, out: []Range{

			// A compressed run followed by sparse run.
			{0, 16 * 0x400, false},
			{16 * 0x400, 16 * 0x400, false},
		}},
	}
)

func TestNewCompressedRunReader(t *testing.T) {
	for _, testcase := range TestCases {
		runs := NewCompressedRangeReader(
			testcase.input, 1024, nil, 16)
		assert.Equal(t, testcase.out, runs.runs)
	}
}

func TestReaderRanges(t *testing.T) {
	for _, testcase := range ReaderTestCases {
		runs := NewCompressedRangeReader(
			testcase.input, 1024, nil, 16)
		assert.Equal(t, testcase.out, runs.Ranges())
	}
}
