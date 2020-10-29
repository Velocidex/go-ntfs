package main

import (
	"fmt"
	"io"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	runs_command = app.Command(
		"runs", "Display sparse runs.")

	record_directory = app.Flag(
		"record", "Path to read/write recorded data").
		Default("").String()

	runs_command_file_arg = runs_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	runs_command_arg = runs_command.Arg(
		"mft_id", "An inode in MFT notation e.g. 43-128-0.",
	).Required().String()
)

func getReader(reader io.ReaderAt) io.ReaderAt {
	if *record_directory == "" {
		return reader
	}

	// Create a recorder
	parser.Printf("Will record to dir %v\n", *record_directory)
	return parser.NewRecorder(*record_directory, reader)
}

func doRuns() {
	reader, _ := parser.NewPagedReader(
		getReader(*runs_command_file_arg), 1024, 10000)
	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	// Access by mft id (e.g. 1234-128-6)
	mft_id, attr_type, attr_id, err := parser.ParseMFTId(*runs_command_arg)
	kingpin.FatalIfError(err, "Can not ParseMFTId")

	mft_entry, err := ntfs_ctx.GetMFT(mft_id)
	kingpin.FatalIfError(err, "Can not open path")

	data, err := parser.OpenStream(ntfs_ctx, mft_entry,
		uint64(attr_type), uint16(attr_id))
	kingpin.FatalIfError(err, "Can not open stream")

	fmt.Println(parser.DebugString(data, ""))

	for _, rng := range data.Ranges() {
		fmt.Printf("Range %v-%v sparse %v\n",
			rng.Offset, rng.Offset+rng.Length, rng.IsSparse)
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
