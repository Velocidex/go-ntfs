package main

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/olekukonko/tablewriter"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	ls_command = app.Command(
		"ls", "List files.")

	ls_command_file_arg = ls_command.Arg(
		"file", "The image file to inspect",
	).Required().OpenFile(os.O_RDONLY, os.FileMode(0666))

	ls_command_arg = ls_command.Arg(
		"path", "The path to list or an MFT entry.",
	).Default("/").String()

	ls_command_image_offset = ls_command.Flag(
		"image_offset", "An offset into the file.",
	).Default("0").Int64()

	mft_regex = regexp.MustCompile("\\d+")
)

func doLS() {
	reader, _ := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *ls_command_image_offset,
		Reader: getReader(*ls_command_file_arg),
	}, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	dir, err := GetMFTEntry(ntfs_ctx, *ls_command_arg)
	kingpin.FatalIfError(err, "Can not open path")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"MFT Id",
		"FullPath",
		"Size",
		"Mtime",
		"IsDir",
		"Filename",
	})
	table.SetCaption(true, fmt.Sprintf(
		"Directory listing for MFT %v", *ls_command_arg))
	defer table.Render()

	for _, info := range parser.ListDir(ntfs_ctx, dir) {

		child_entry, err := GetMFTEntry(ntfs_ctx, info.MFTId)
		kingpin.FatalIfError(err, "Can not open child %v", info.Name)

		full_path := parser.GetFullPath(ntfs_ctx, child_entry)

		table.Append([]string{
			info.MFTId,
			full_path,
			fmt.Sprintf("%v", info.Size),
			fmt.Sprintf("%v", info.Mtime.In(time.UTC)),
			fmt.Sprintf("%v", info.IsDir),
			info.Name,
		})
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "ls":
			doLS()
		default:
			return false
		}
		return true
	})
}
