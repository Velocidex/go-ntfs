package main

import (
	"fmt"
	"os/exec"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	test_fn_command = app.Command(
		"test_filename", "Test file path reconstruction by comparing with TSK.")

	test_fn_command_file_arg = test_fn_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	test_fn_command_ffind_path = test_fn_command.Flag(
		"ffind_path", "Path to the ffind binary").
		Default("ffind").String()

	test_fn_command_start_id = test_fn_command.Flag(
		"start", "First MFT ID to test").Default("0").Int64()
)

func calcFilenameWithTSK(inode string) (string, error) {
	command := exec.Command(*test_fn_command_ffind_path,
		(*test_fn_command_file_arg).Name(), inode)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func doTestFn() {
	reader, err := parser.NewPagedReader(*test_fn_command_file_arg, 1024, 10000)
	kingpin.FatalIfError(err, "Can not open MFT file")

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_stream, err := parser.GetDataForPath(ntfs_ctx, "$MFT")
	kingpin.FatalIfError(err, "Can not open filesystem")

	for i := int64(*test_command_start_id); i < parser.RangeSize(mft_stream)/ntfs_ctx.RecordSize; i++ {
		mft_entry, err := ntfs_ctx.GetMFT(i)
		if err != nil {
			continue
		}

		full_path, err := parser.GetFullPath(ntfs_ctx, mft_entry)
		if err != nil {
			fmt.Printf("Error %v: %v\n", i, err)
			continue
		}

		if *verbose_flag {
			fmt.Printf("%v: %v\n", i, full_path)
		}

		tsk_path, err := calcFilenameWithTSK(fmt.Sprintf("%v", i))
		if err != nil {
			fmt.Printf("go-ntfs Error %v: %v\n", i, err)
		}

		if *verbose_flag {
			fmt.Printf("tsk_path %v: %v\n", i, tsk_path)
		}

		if tsk_path != full_path {
			fmt.Printf("**** ERROR %v: %v != %v\n", i, tsk_path, full_path)
		}

	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case test_fn_command.FullCommand():
			doTestFn()
		default:
			return false
		}
		return true
	})
}
