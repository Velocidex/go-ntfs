package main

import (
	"context"
	"fmt"
	"strings"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	carve_command = app.Command(
		"carve", "Carve USN records from the disk.")

	carve_command_file_arg = carve_command.Arg(
		"file", "The image file to inspect",
	).Required().File()
)

const carve_template = `
USN ID: %#x @ %d
Filename: %s
FullPath: %s
Timestamp: %v
Reason: %s
FileAttributes: %s
SourceInfo: %s
`

func doCarve() {
	reader, _ := parser.NewPagedReader(
		getReader(*carve_command_file_arg), 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	size := ntfs_ctx.Boot.VolumeSize() * int64(ntfs_ctx.Boot.Sector_size())
	fmt.Printf("VolumeSize %v\n", size)

	for record := range parser.CarveUSN(
		context.Background(), ntfs_ctx, reader, size) {

		filename := record.Filename()

		fmt.Printf(carve_template, record.Usn(), record.DiskOffset,
			filename,
			record.Links(), record.TimeStamp(),
			strings.Join(record.Reason(), ", "),
			strings.Join(record.FileAttributes(), ", "),
			strings.Join(record.SourceInfo(), ", "),
		)
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case carve_command.FullCommand():
			doCarve()
		default:
			return false
		}
		return true
	})
}
