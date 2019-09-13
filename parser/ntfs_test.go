package parser_test

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/sebdah/goldie"
	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/go-ntfs/parser"
)

func split(message string) interface{} {
	if !strings.Contains(message, "\n") {
		return message
	}

	return strings.Split(message, "\n")
}

func TestNTFS(t *testing.T) {
	result := make(map[string]interface{})
	assert := assert.New(t)

	fd, err := os.Open("test_data/test.ntfs.dd")
	assert.NoError(err, "Unable to open file")

	ntfs_ctx, err := parser.GetNTFSContext(fd, 0)
	assert.NoError(err, "Unable to open file")

	// Open directory by path.
	root, err := ntfs_ctx.GetMFT(5)
	assert.NoError(err, "Unable to open file")

	dir, err := root.Open(ntfs_ctx, "Folder A/Folder B")
	assert.NoError(err, "Open by path")

	result["01 Open by path"] = parser.ListDir(ntfs_ctx, dir)
	result["02 Folder B stat"] = split(dir.DebugString())
	result["03 I30"] = parser.ExtractI30List(ntfs_ctx, dir)

	// Open by mft id
	mft_idx, attr, id, err := parser.ParseMFTId("46-128-5")
	assert.NoError(err, "ParseMFTId")
	assert.Equal(mft_idx, int64(46))
	assert.Equal(attr, int64(128))
	assert.Equal(id, int64(5))

	buf := make([]byte, 3000000)
	reader, err := parser.GetDataForPath(ntfs_ctx,
		"Folder A/Folder B/Hello world text document.txt")
	assert.NoError(err, "GetDataForPath")

	n, _ := reader.ReadAt(buf, 0)
	result["04 Hello world.txt"] = fmt.Sprintf("%v: %s", n, string(buf[:n]))

	reader, err = parser.GetDataForPath(ntfs_ctx,
		"Folder A/Folder B/Hello world text document.txt:goodbye.txt")
	assert.NoError(err, "GetDataForPath ADS")

	n, _ = reader.ReadAt(buf, 0)
	result["05 Hello world.txt:goodbye.txt"] = fmt.Sprintf(
		"%v: %s", n, string(buf[:n]))

	reader, err = parser.GetDataForPath(ntfs_ctx, "ones.bin")
	assert.NoError(err, "Open compressed ones.bin")

	n, _ = reader.ReadAt(buf, 0)
	h := sha1.New()
	h.Write(buf[:n])
	result["06 Compressed ones.bin hash"] = fmt.Sprintf(
		"%v: %s", n, hex.EncodeToString(h.Sum(nil)))

	result_json, _ := json.MarshalIndent(result, "", " ")
	goldie.Assert(t, "TestNTFS", result_json)
}

func init() {
	time.Local = time.UTC
	spew.Config.DisablePointerAddresses = true
	spew.Config.SortKeys = true
}
