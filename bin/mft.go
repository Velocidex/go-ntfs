package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	mft_command = app.Command(
		"mft", "Process a raw $MFT file.")

	mft_command_file_arg = mft_command.Flag(
		"file", "The $MFT file to process",
	).File()

	mft_command_image_arg = mft_command.Flag(
		"image", "An image containing an $MFT",
	).File()

	mft_command_image_offset = mft_command.Flag(
		"image_offset", "The offset in the image to use.",
	).Int64()

	mft_command_filename_filter = mft_command.Flag(
		"filename_filter", "A regex to filter on filename",
	).Default(".").String()
)

const (
	mft_entry_size int64 = 0x400
)

type DetailedHighlights struct {
	*parser.MFTHighlight
	FullPath string
	Links    []string
}

func doMFTFromFile() {
	reader, _ := parser.NewPagedReader(*mft_command_file_arg, 1024, 10000)
	st, err := (*mft_command_file_arg).Stat()
	kingpin.FatalIfError(err, "Can not open MFT file")

	for item := range parser.ParseMFTFile(context.Background(),
		reader, st.Size(), 0x1000, 0x400) {
		serialized, err := json.MarshalIndent(item, " ", " ")
		kingpin.FatalIfError(err, "Marshal")

		fmt.Println(string(serialized))
	}
}

func doMFTFromImage() {
	filename_filter := regexp.MustCompile(*mft_command_filename_filter)

	reader, err := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *mft_command_image_offset,
		Reader: getReader(*mft_command_image_arg),
	}, 0x400, 10000)
	kingpin.FatalIfError(err, "Can not open image")

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	// Get the MFT ID 0
	mft_entry, err := GetMFTEntry(ntfs_ctx, "0")
	kingpin.FatalIfError(err, "Can not open path")

	// Get the first data stream of ID 0 - this is the MFT
	mft_reader, err := parser.OpenStream(ntfs_ctx, mft_entry,
		uint64(128), parser.WILDCARD_STREAM_ID,
		parser.WILDCARD_STREAM_NAME)
	kingpin.FatalIfError(err, "Can not open stream")

	for item := range parser.ParseMFTFile(context.Background(),
		mft_reader, parser.RangeSize(mft_reader),
		ntfs_ctx.ClusterSize, ntfs_ctx.RecordSize) {
		if len(filename_filter.FindStringIndex(item.FileName())) == 0 {
			continue
		}

		serialized, err := json.MarshalIndent(DetailedHighlights{
			MFTHighlight: item,
			FullPath:     item.FullPath(),
			Links:        item.Links(),
		}, " ", " ")
		kingpin.FatalIfError(err, "Marshal")

		fmt.Println(string(serialized))
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "mft":
			if *mft_command_file_arg != nil {
				doMFTFromFile()
			} else if *mft_command_image_arg != nil {
				doMFTFromImage()
			}
		default:
			return false
		}
		return true
	})
}
