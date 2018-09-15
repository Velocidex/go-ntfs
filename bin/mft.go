package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs"
)

var (
	mft_command = app.Command(
		"mft", "inspect the boot record.")

	mft_command_file_arg = mft_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	mft_command_arg = mft_command.Arg(
		"MFT", "The MFT ID to list (5 is the root).",
	).Default("5").Int64()
)

func doMFT() {
	profile, err := ntfs.GetProfile()
	kingpin.FatalIfError(err, "Profile")

	boot, err := ntfs.NewBootRecord(profile, *mft_command_file_arg, 0)
	kingpin.FatalIfError(err, "Boot record")

	mft, err := boot.MFT()
	kingpin.FatalIfError(err, "MFT")

	root, err := mft.MFTEntry(*mft_command_arg)
	kingpin.FatalIfError(err, "Root")

	fmt.Println(root.DebugString())
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "mft":
			doMFT()
		default:
			return false
		}
		return true
	})
}
