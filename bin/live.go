package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"strings"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	live_test_command = app.Command(
		"live_test", "Test file reading by performing live hashing.")

	live_test_command_drive = live_test_command.Arg(
		"drive", "Drive letter to test").
		Default("C").String()

	live_test_command_glob = live_test_command.Arg(
		"glob", "Starting path to recurse").
		Default("Windows\\*").String()

	live_test_command_profile = live_test_command.Flag(
		"profile", "Write profile data to this filename").String()
)

func doLiveTest() {
	now := time.Now()
	defer func() {
		fmt.Printf("Completed test in %v\n", time.Now().Sub(now))
	}()

	if *live_test_command_profile != "" {
		f2, err := os.Create(*live_test_command_profile)
		kingpin.FatalIfError(err, "Creating Profile file.")
		err = pprof.StartCPUProfile(f2)
		kingpin.FatalIfError(err, "Profile file.")
		defer pprof.StopCPUProfile()
	}

	matched, _ := regexp.MatchString("^[a-zA-Z]$", *live_test_command_drive)
	if !matched {
		kingpin.Fatalf("Invalid drive letter: should be eg 'C'")
	}

	image_fd, err := os.Open("\\\\.\\" + *live_test_command_drive + ":")
	kingpin.FatalIfError(err, "Can not open drive")

	reader, _ := parser.NewPagedReader(image_fd, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	glob := fmt.Sprintf("%s:\\%s", *live_test_command_drive,
		*live_test_command_glob)

	files, err := filepath.Glob(glob)
	kingpin.FatalIfError(err, "Can not glob files")

	for _, filename := range files {
		stat, err := os.Lstat(filename)
		if err != nil {
			kingpin.FatalIfError(err, "Can not glob files")

			continue
		}

		if stat.IsDir() {
			continue
		}

		fd, err := os.Open(filename)
		if err != nil {
			continue
		}

		// Calculate the hash of the file
		h := sha256.New()

		buf := make([]byte, 1024*1024)

		_, err = io.CopyBuffer(h, fd, buf)
		if err != nil && err != io.EOF {
			continue
		}

		os_hash := fmt.Sprintf("File %v %x\n", filename, h.Sum(nil))

		mft_entry, err := GetMFTEntry(ntfs_ctx, getRelativePath(filename))
		if err != nil {
			fmt.Printf("ERROR Getting MFT Id: %v\n", err)
		}

		reader, err := parser.OpenStream(ntfs_ctx, mft_entry, 128, 0)
		if err != nil {
			fmt.Printf("ERROR Getting MFT Id %v: %v\n",
				mft_entry.Record_number(), err)
		}

		ntfs_h := sha256.New()
		_, err = io.CopyBuffer(ntfs_h, &readAdapter{reader: reader}, buf)
		if err != nil && err != io.EOF {
			fmt.Printf("ERROR Getting MFT Id %v: %v\n",
				mft_entry.Record_number(), err)
			continue
		}

		ntfs_hash := fmt.Sprintf("File %v %x\n", filename, h.Sum(nil))
		if os_hash != ntfs_hash {
			fmt.Printf("ERROR Mismatch hash on MFT entry %v:\n%s\n%s\n",
				mft_entry.Record_number(), os_hash, ntfs_hash)
		} else {
			fmt.Printf("%v: %v", time.Now().Format(time.RFC3339), os_hash)
		}
	}
}

func getRelativePath(filename string) string {
	parts := strings.SplitN(filename, ":", 2)
	if len(parts) > 0 {
		return parts[1]
	}
	return filename
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case live_test_command.FullCommand():
			doLiveTest()
		default:
			return false
		}
		return true
	})
}
