package main

import (
	"encoding/json"
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	stat_command = app.Command(
		"stat", "inspect the MFT record.")

	stat_command_i30 = stat_command.Flag(
		"i30", "Carve out $I30 entries").Bool()

	stat_command_file_arg = stat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	stat_command_image_offset = stat_command.Flag(
		"image_offset", "The offset in the image to use.",
	).Int64()

	stat_command_arg = stat_command.Arg(
		"path", "The path to list or an STAT entry.",
	).Default("5").String()
)

func doSTAT() {
	reader, _ := parser.NewPagedReader(&parser.OffsetReader{
		Offset: *stat_command_image_offset,
		Reader: getReader(*stat_command_file_arg),
	}, 1024, 10000)

	ntfs_ctx, err := parser.GetNTFSContext(reader, 0)
	kingpin.FatalIfError(err, "Can not open filesystem")

	mft_entry, err := GetMFTEntry(ntfs_ctx, *stat_command_arg)
	kingpin.FatalIfError(err, "Can not open path")

	if *verbose_flag {
		fmt.Println(mft_entry.Display(ntfs_ctx))

	} else {
		stat, err := parser.ModelMFTEntry(ntfs_ctx, mft_entry)
		kingpin.FatalIfError(err, "Can not open path")

		serialized, err := json.MarshalIndent(stat, " ", " ")
		kingpin.FatalIfError(err, "Marshal")

		fmt.Println(string(serialized))
	}

	if *stat_command_i30 {
		i30_list := parser.ExtractI30List(ntfs_ctx, mft_entry)
		kingpin.FatalIfError(err, "Can not extract $I30")

		serialized, err := json.MarshalIndent(i30_list, " ", " ")
		kingpin.FatalIfError(err, "Marshal")

		fmt.Println(string(serialized))
	}
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
