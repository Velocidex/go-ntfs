package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"

	"github.com/olekukonko/tablewriter"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	ntfs "www.velocidex.com/golang/go-ntfs"
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

	mft_regex = regexp.MustCompile("\\d+")
)

func getMFTEntry(path_or_entry string, image io.ReaderAt) (
	*ntfs.MFT_ENTRY, error) {
	profile, err := ntfs.GetProfile()
	kingpin.FatalIfError(err, "Profile")

	boot, err := ntfs.NewBootRecord(profile, image, 0)
	kingpin.FatalIfError(err, "Boot record")

	mft, err := boot.MFT()
	kingpin.FatalIfError(err, "MFT")

	if mft_regex.MatchString(path_or_entry) {
		mft_entry, err := strconv.Atoi(path_or_entry)
		kingpin.FatalIfError(err, "MFT entry not valid")

		return mft.MFTEntry(int64(mft_entry))
	} else {
		root, err := mft.MFTEntry(5)
		kingpin.FatalIfError(err, "MFT")

		return root.Open(path_or_entry)
	}
}

func doLS() {
	dir, err := getMFTEntry(*ls_command_arg, *ls_command_file_arg)
	kingpin.FatalIfError(err, "Can not open directory MFT entry.")

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"MFT Id",
		"Size",
		"Mtime",
		"IsDir",
		"Filename",
	})
	table.SetCaption(true, fmt.Sprintf(
		"Directory listing for MFT %v", *ls_command_arg))
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
		case "ls":
			doLS()
		default:
			return false
		}
		return true
	})
}
