package main

import (
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	ntfs "www.velocidex.com/golang/go-ntfs"
)

var (
	fls_command = app.Command(
		"fls", "List files.")

	fls_command_file_arg = fls_command.Arg(
		"file", "The image file to inspect",
	).Required().OpenFile(os.O_RDONLY, os.FileMode(0666))

	fls_command_arg = fls_command.Arg(
		"MFT", "The MFT ID to list (5 is the root).",
	).Default("5").Int64()
)

func doFLS() {
	profile, err := ntfs.GetProfile()
	kingpin.FatalIfError(err, "Profile")

	boot, err := ntfs.NewBootRecord(profile, *fls_command_file_arg, 0)
	kingpin.FatalIfError(err, "Boot record")

	mft, err := boot.MFT()
	kingpin.FatalIfError(err, "MFT")

	dir, err := mft.MFTEntry(*fls_command_arg)
	kingpin.FatalIfError(err, "Root")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"MFT Id",
		"Size",
		"Mtime",
		"IsDir",
		"Filename",
	})
	table.SetCaption(true, fmt.Sprintf(
		"Directory listing for MFT %d", *fls_command_arg))
	defer table.Render()

	for _, node := range dir.Dir() {
		node_mft_id := node.Get("mftReference").AsInteger()
		node_mft, err := dir.MFTEntry(node_mft_id)
		if err != nil {
			continue
		}

		table.Append([]string{
			fmt.Sprintf("%d",
				node.Get("mftReference").AsInteger()),
			node.Get("file.size").AsString(),
			node.Get("file.file_modified").AsString(),
			fmt.Sprintf("%v", node_mft.IsDir()),
			(&ntfs.FILE_NAME{node.Get("file")}).Name(),
		})
	}

}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "fls":
			doFLS()
		default:
			return false
		}
		return true
	})
}
