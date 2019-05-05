package ntfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testCase struct {
	input []Run
	out   []ReaderRun
}

var (
	TestCases = []testCase{
		{input: []Run{
			{474540, 47},
			{0, 1},
			{48, 1213},
			{0, 3},
		}, out: []ReaderRun{
			{0, 474540, 32, 0},
			{32, 474572, 16, 15},
			{48, 474588, 1200, 0},
			{1248, 475788, 16, 13},
		}},
		// A compressed run followed by a sparse run longer
		// than compression size.
		{input: []Run{
			{1940823, 2},
			{0, 30}, // This is really {0, 14}, {0, 16} merged together.
		}, out: []ReaderRun{

			// A compressed run followed by sparse run.
			{0, 1940823, 16, 2},
			{2, 0, 16, 0},
		}},
	}
)

func TestNewCompressedRunReader(t *testing.T) {
	for _, testcase := range TestCases {
		runs := NewCompressedRunReader(testcase.input, nil, 16)
		assert.Equal(t, testcase.out, runs.runs)
	}
}
