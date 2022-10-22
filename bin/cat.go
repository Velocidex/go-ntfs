package main

import (
	"io"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	cat_command = app.Command(
		"cat", "Dump file stream.")

	cat_command_file_arg = cat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	cat_command_arg = cat_command.Arg(
		"path", "The path to extract to an MFT entry.",
	).Default("/").String()

	cat_command_offset = cat_command.Flag(
		"offset", "The offset to start reading.",
	).Int64()

	cat_command_image_offset = cat_command.Flag(
		"image_offset", "The offset in the image to use.",
	).Int64()

	cat_command_output_file = cat_command.Flag(
		"out", "Write to this file",
	).OpenFile(os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(0666))
)

func doCAT() {
	reader, _ := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *cat_command_image_offset,
		Reader: getReader(*cat_command_file_arg),
	}, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_entry, err := GetMFTEntry(ntfs_ctx, *cat_command_arg)
	kingpin.FatalIfError(err, "Can not open path")

	// Access by mft id (e.g. 1234-128-6)
	_, attr_type, attr_id, err := parser.ParseMFTId(*cat_command_arg)
	if err != nil {
		attr_type = 128 // $DATA
	}

	data, err := parser.OpenStream(ntfs_ctx, mft_entry,
		uint64(attr_type), uint16(attr_id))
	kingpin.FatalIfError(err, "Can not open stream")

	var fd io.WriteCloser = os.Stdout
	if *cat_command_output_file != nil {
		fd = *cat_command_output_file
		defer fd.Close()
	}

	buf := make([]byte, 1024*1024*10)
	offset := *cat_command_offset
	for {
		n, _ := data.ReadAt(buf, offset)
		if n == 0 {
			return
		}
		fd.Write(buf[:n])
		offset += int64(n)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "cat":
			doCAT()
		default:
			return false
		}
		return true
	})
}
