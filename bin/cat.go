package main

import (
	"io"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	ntfs "www.velocidex.com/golang/go-ntfs"
)

var (
	cat_command = app.Command(
		"cat", "List files.")

	cat_command_file_arg = cat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	cat_command_arg = cat_command.Arg(
		"path", "The path to extract to an MFT entry.",
	).Default("/").String()

	cat_command_output_file = cat_command.Flag(
		"out", "Write to this file",
	).OpenFile(os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(0666))
)

func doCAT() {
	reader, _ := ntfs.NewPagedReader(*cat_command_file_arg, 1024, 10000)
	root, err := ntfs.GetRootMFTEntry(reader)
	kingpin.FatalIfError(err, "Can not open filesystem")

	var data io.ReaderAt
	mft_idx, attr_type, attr_id, err := ntfs.ParseMFTId(*cat_command_arg)
	if err == nil {
		// Access by mft id (e.g. 1234-128-6)
		mft_entry, err := root.MFTEntry(mft_idx)
		kingpin.FatalIfError(err, "Can not open root MFT entry")
		data = mft_entry.Data(attr_type, attr_id)
	} else {
		// Access by filename - retrieve the first unnamed
		// $DATA stream.
		data, err = ntfs.GetDataForPath(*cat_command_arg, root)
		kingpin.FatalIfError(err, "Can not open path")
	}

	var fd io.WriteCloser = os.Stdout

	if *cat_command_output_file != nil {
		fd = *cat_command_output_file
		defer fd.Close()
	}

	buf := make([]byte, 1024*1024)
	offset := int64(0)
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
