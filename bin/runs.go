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

	runs_command_image_offset = runs_command.Flag(
		"image_offset", "The offset in the image to use.",
	).Int64()

	runs_command_raw_runs = runs_command.Flag(
		"raw_runs", "Also show raw runs.",
	).Bool()

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
	reader, _ := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *runs_command_image_offset,
		Reader: getReader(*runs_command_file_arg),
	}, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_entry, err := GetMFTEntry(ntfs_ctx, *runs_command_arg)
	kingpin.FatalIfError(err, "Can not open path")

	// Access by mft id (e.g. 1234-128-6) or filepath (e.g. C:\Folder\Hello.txt:hiddenstream)
	_, attr_type, attr_id, ads_name, err := parser.ParseMFTId(*runs_command_arg)
	if err != nil {
		attr_type = 128 // $DATA
	}

	if *runs_command_raw_runs {
		vcns := parser.GetAllVCNs(ntfs_ctx,
			mft_entry, uint64(attr_type), uint16(attr_id), ads_name)
		for _, vcn := range vcns {
			fmt.Println(vcn.DebugString())
			vcn_runlist := vcn.RunList()
			parser.DebugRawRuns(vcn_runlist)
		}
	}

	data, err := parser.OpenStream(ntfs_ctx, mft_entry,
		uint64(attr_type), uint16(attr_id), ads_name)
	kingpin.FatalIfError(err, "Can not open stream")

	for idx, r := range parser.DebugRuns(data, 0) {
		fmt.Printf("%d %v\n", idx, r)
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
