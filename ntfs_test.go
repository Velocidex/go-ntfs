package ntfs_test

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
	ntfs "www.velocidex.com/golang/go-ntfs"
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

	root, err := ntfs.GetRootMFTEntry(fd)
	assert.NoError(err, "Unable to open file")

	// Open directory by path.
	dir, err := root.Open("Folder A/Folder B")
	assert.NoError(err, "Open by path")

	result["01 Open by path"] = ntfs.ListDir(dir)
	result["02 Folder B stat"] = split(dir.DebugString())
	result["02.1 I30"] = ntfs.ExtractI30List(dir)

	// Open by mft id
	mft_idx, attr, id, err := ntfs.ParseMFTId("46-128-5")
	assert.NoError(err, "ParseMFTId")
	assert.Equal(mft_idx, int64(46))
	assert.Equal(attr, int64(128))
	assert.Equal(id, int64(5))

	buf := make([]byte, 3000000)
	reader, err := ntfs.GetDataForPath(
		"Folder A/Folder B/Hello world text document.txt", root)
	assert.NoError(err, "GetDataForPath")

	n, _ := reader.ReadAt(buf, 0)
	result["03 Hello world.txt"] = fmt.Sprintf("%v: %s", n, string(buf[:n]))

	reader, err = ntfs.GetDataForPath(
		"Folder A/Folder B/Hello world text document.txt:goodbye.txt",
		root)
	assert.NoError(err, "GetDataForPath ADS")

	n, _ = reader.ReadAt(buf, 0)
	result["04 Hello world.txt:goodbye.txt"] = fmt.Sprintf(
		"%v: %s", n, string(buf[:n]))

	reader, err = ntfs.GetDataForPath("ones.bin", root)
	assert.NoError(err, "Open compressed ones.bin")

	n, _ = reader.ReadAt(buf, 0)
	h := sha1.New()
	h.Write(buf[:n])
	result["05 Compressed ones.bin hash"] = fmt.Sprintf(
		"%v: %s", n, hex.EncodeToString(h.Sum(nil)))

	result_json, _ := json.MarshalIndent(result, "", " ")
	goldie.Assert(t, "TestNTFS", result_json)
}

func init() {
	time.Local = time.UTC
	spew.Config.DisablePointerAddresses = true
	spew.Config.SortKeys = true
}
