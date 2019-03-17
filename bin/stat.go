package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	ntfs "www.velocidex.com/golang/go-ntfs"
)

var (
	stat_command = app.Command(
		"stat", "inspect the boot record.")

	stat_command_file_arg = stat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	stat_command_arg = stat_command.Arg(
		"path", "The path to list or an STAT entry.",
	).Default("5").String()
)

func doSTAT() {
	reader, _ := ntfs.NewPagedReader(*stat_command_file_arg, 1024, 10000)
	root, err := ntfs.GetRootMFTEntry(reader)
	kingpin.FatalIfError(err, "Can not open filesystem")

	var mft_entry *ntfs.MFT_ENTRY

	mft_idx, _, _, err := ntfs.ParseMFTId(*stat_command_arg)
	if err == nil {
		// Access by mft id (e.g. 1234-128-6)
		mft_entry, err = root.MFTEntry(mft_idx)
	} else {
		// Access by filename - retrieve the first unnamed
		// $DATA stream.
		mft_entry, err = root.Open(*stat_command_arg)
	}
	kingpin.FatalIfError(err, "Can not open path")

	fmt.Println(mft_entry.DebugString())
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "stat":
			doSTAT()
		default:
			return false
		}
		return true
	})
}
