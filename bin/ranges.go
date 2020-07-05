package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	runs_command = app.Command(
		"runs", "Display sparse runs.")

	runs_command_file_arg = runs_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	runs_command_arg = runs_command.Arg(
		"path", "The path to extract to an MFT entry.",
	).Default("/").String()
)

func doRuns() {
	reader, _ := parser.NewPagedReader(*runs_command_file_arg, 1024, 10000)
	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_entry, err := GetMFTEntry(ntfs_ctx, *runs_command_arg)
	kingpin.FatalIfError(err, "Can not open path")

	// Access by mft id (e.g. 1234-128-6)
	_, attr_type, attr_id, err := parser.ParseMFTId(*runs_command_arg)
	if err != nil {
		attr_type = 128 // $DATA
	}

	data, err := parser.OpenStream(ntfs_ctx, mft_entry,
		uint64(attr_type), uint16(attr_id))
	kingpin.FatalIfError(err, "Can not open stream")

	fmt.Println(parser.DebugString(data, ""))

	for _, rng := range data.Ranges() {
		fmt.Printf("Range %v-%v sparse %v\n", rng.Offset, rng.Length, rng.IsSparse)
	}

}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "runs":
			doRuns()
		default:
			return false
		}
		return true
	})
}
