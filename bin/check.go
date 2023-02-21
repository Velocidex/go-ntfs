package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	check_command = app.Command(
		"check", "Check for some sanity.")

	check_command_file_arg = check_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	check_command_image_offset = check_command.Flag(
		"image_offset", "The offset in the image to use.",
	).Int64()

	check_command_start_id = check_command.Flag(
		"start", "The ID to start with").Int64()

	check_command_end_id = check_command.Flag(
		"end", "The ID to end with").Default("10000000").Int64()
)

func doCheck() {
	reader, _ := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *check_command_image_offset,
		Reader: getReader(*check_command_file_arg),
	}, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	for i := *check_command_start_id; i < *check_command_end_id; i++ {
		reportError(ntfs_ctx, i)

		mft_entry, err := ntfs_ctx.GetMFT(i)
		if err != nil {
			fmt.Printf("Error: %v: %v\n", i, err)
			continue
		}

		stats := parser.Stat(ntfs_ctx, mft_entry)
		if len(stats) == 0 {
			continue
		}

		if i%100 == 0 {
			fmt.Printf("Getting id %v - %v %v\n", i, mft_entry.Record_number(),
				stats[0].Name)
		}

		if mft_entry.Record_number() != uint32(i) {
			panic(i)
		}
	}
}

func reportError(ntfs_ctx *parser.NTFSContext, id int64) {
	offset := ntfs_ctx.GetRecordSize() * id
	disk_offset := parser.VtoP(ntfs_ctx.MFTReader, offset)
	fmt.Printf("MFTId %v: Offset %v (%v clusters), disk offset %v (%v clusters)\n",
		id, offset, offset/ntfs_ctx.ClusterSize,
		disk_offset, disk_offset/ntfs_ctx.ClusterSize)
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "check":
			doCheck()
		default:
			return false
		}
		return true
	})
}
