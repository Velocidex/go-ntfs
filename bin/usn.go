package main

import (
	"context"
	"fmt"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	usn_command = app.Command(
		"usn", "inspect the USN journal.")

	usn_command_file_arg = usn_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	usn_command_watch = usn_command.Flag(
		"watch", "Watch the USN for changes").Bool()
)

const template = `
USN ID: %x @ %x
Filename: %s
FullPath: %s
Timestamp: %v
Reason: %s
FileAttributes: %s
SourceInfo: %s
`

func doWatchUSN() {
	reader, _ := parser.NewPagedReader(
		getReader(*usn_command_file_arg), 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	for record := range parser.WatchUSN(context.Background(), ntfs_ctx, 1) {
		fmt.Printf(template, record.Usn(), record.Offset,
			record.Filename(),
			record.FullPath(), record.TimeStamp(),
			strings.Join(record.Reason(), ", "),
			strings.Join(record.FileAttributes(), ", "),
			strings.Join(record.SourceInfo(), ", "),
		)
	}
}

func doUSN() {
	if *usn_command_watch {
		doWatchUSN()
		return
	}

	reader, _ := parser.NewPagedReader(
		getReader(*usn_command_file_arg), 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	for record := range parser.ParseUSN(context.Background(), ntfs_ctx, 0) {
		fmt.Printf(template, record.Usn(), record.Offset,
			record.Filename(),
			record.FullPath(), record.TimeStamp(),
			strings.Join(record.Reason(), ", "),
			strings.Join(record.FileAttributes(), ", "),
			strings.Join(record.SourceInfo(), ", "),
		)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case usn_command.FullCommand():
			doUSN()
		default:
			return false
		}
		return true
	})
}
