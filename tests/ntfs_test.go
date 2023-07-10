package ntfs

/*
  This test suite is designed to trap regressions in the NTFS parser.
  It relies on actual real life observed NTFS filesystems that were
  encountered in the past. For each interesting testcase, we used the
  recorder to capture the disk sectors involved in the specific
  operation under test and then the test replays these sectors under
  test conditions. This allows us to replicate exactly the image that
  was encountered in the wild - without having to maintain a large
  volume of disk images.

  The main goal of the test is to confirm the parser produces correct
  results and does not break in the future. Therefore we create a set
  of golden files which are then compared with the tool output on
  these test cases.
*/

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/suite"
)

type NTFSTestSuite struct {
	suite.Suite
	binary, extension string
	tmpdir            string
}

func (self *NTFSTestSuite) SetupTest() {
	if runtime.GOOS == "windows" {
		self.extension = ".exe"
	}

	// Search for a valid binary to run.
	binaries, err := filepath.Glob(
		"../ntfs" + self.extension)
	assert.NoError(self.T(), err)

	self.binary, _ = filepath.Abs(binaries[0])
	fmt.Printf("Found binary %v\n", self.binary)

	self.tmpdir, err = ioutil.TempDir("", "tmp")
	assert.NoError(self.T(), err)
}

func (self *NTFSTestSuite) TearDownTest() {
	os.RemoveAll(self.tmpdir)
}

// This test case looks at a file which has a sparse ending: The file
// is 1048576 bytes long but has only 4096 initializaed with real
// data - the rest is sparse.
func (self *NTFSTestSuite) TestLargeFileSmallInit() {
	record_dir := "large_file_small_init"
	cmd := exec.Command(self.binary, "--record", record_dir,
		"runs", self.binary, "46", "--verbose")
	out, err := cmd.CombinedOutput()
	assert.NoError(self.T(), err, string(out))

	g := goldie.New(self.T(), goldie.WithFixtureDir(record_dir+"/fixtures"))
	g.Assert(self.T(), "runs", out)

	// Make sure we write the right length of data.
	dd_file := filepath.Join(self.tmpdir, "cat.dd")
	cmd = exec.Command(self.binary, "--record", record_dir,
		"cat", self.binary, "46", "--out", dd_file)
	_, err = cmd.CombinedOutput()
	assert.NoError(self.T(), err)

	s, err := os.Lstat(dd_file)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), s.Size(), int64(1048576))
}

// This test case looks at a sparse $J USN journal with two VCNs.
func (self *NTFSTestSuite) TestUSNWith2VCNs() {
	record_dir := "usn_with_two_vcns"
	cmd := exec.Command(self.binary, "--record",
		record_dir, "runs", self.binary, "68310:$J", "--verbose")
	out, err := cmd.CombinedOutput()
	assert.NoError(self.T(), err, string(out))

	g := goldie.New(self.T(), goldie.WithFixtureDir(record_dir+"/fixtures"))
	g.Assert(self.T(), "runs", out)

	// Make sure we write the right length of data.
	cmd = exec.Command(self.binary, "--record", record_dir,
		"stat", self.binary, "68310")
	cmd.Env = append(os.Environ(), "TZ=Z")
	out, err = cmd.CombinedOutput()
	assert.NoError(self.T(), err)
	g.Assert(self.T(), "stat", out)
}

// This file contains multiple data streams with the same ID. We need
// to generate Inode strings which include the stream name so it can
// be disambiguated.
func (self *NTFSTestSuite) TestMultipleADSWithSameID() {
	record_dir := "ads_with_same_ids"
	cmd := exec.Command(self.binary, "--record", record_dir,
		"ls", self.binary)
	out_b, err := cmd.CombinedOutput()
	out := string(out_b)
	assert.NoError(self.T(), err, out)

	// Check that inodes do not include ADS unless it is necessary for disambiguation
	g := goldie.New(self.T(), goldie.WithFixtureDir(record_dir+"/fixtures"))
	g.Assert(self.T(), "ls", out_b)

	assert.Contains(self.T(), out, "38-128-0:111")
	assert.Contains(self.T(), out, "38-128-0:333")
	assert.Contains(self.T(), out, "38-128-3 ")

	// $Secure:$SDS is only stream with ADS and it has non zero ID -
	// we dont need an inode with ADS
	assert.Contains(self.T(), out, "9-128-8 ")

	// Search first stream with same id
	cmd = exec.Command(self.binary, "--record", record_dir, "stat", self.binary, "38")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.NoError(self.T(), err, out)
	g = goldie.New(self.T(), goldie.WithFixtureDir(record_dir+"/fixtures"))
	g.Assert(self.T(), "stat", out_b)

	// Search first stream with same id
	cmd = exec.Command(self.binary, "--record", record_dir, "cat", self.binary, "38-128-0")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.NoError(self.T(), err, out)
	assert.Contains(self.T(), out, "1111")

	// Reading by id containing ads
	cmd = exec.Command(self.binary, "--record", record_dir, "cat", self.binary, "38-128-0:111")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.NoError(self.T(), err, out)
	assert.Contains(self.T(), out, "1111")

	// Reading by id containing ads
	cmd = exec.Command(self.binary, "--record", record_dir, "cat", self.binary, "38-128-0:333")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.NoError(self.T(), err, out)
	assert.Contains(self.T(), out, "333")

	// Reading by id of 9 file
	cmd = exec.Command(self.binary, "--record", record_dir, "cat", self.binary, "38-128-3")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.NoError(self.T(), err, out)
	assert.Contains(self.T(), out, "999")

	// Reading by id of 9 file - it does not have any ADS
	cmd = exec.Command(self.binary, "--record", record_dir, "cat", self.binary, "38-128-3:111")
	out_b, err = cmd.CombinedOutput()
	out = string(out_b)
	assert.Error(self.T(), err, out)
	assert.Contains(self.T(), out, "file does not exist")

}

func TestNTFS(t *testing.T) {
	suite.Run(t, &NTFSTestSuite{})
}
