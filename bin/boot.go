package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs"
)

var (
	boot_command = app.Command(
		"boot", "inspect the boot record.")

	boot_command_arg = boot_command.Arg(
		"file", "The image file to inspect",
	).Required().File()
)

func doBoot() {
	profile, err := ntfs.GetProfile()
	kingpin.FatalIfError(err, "Profile")

	boot, err := ntfs.NewBootRecord(profile, *boot_command_arg, 0)
	kingpin.FatalIfError(err, "Boot record")

	mft, err := boot.MFT()
	kingpin.FatalIfError(err, "MFT")

	root, err := mft.MFTEntry(5)
	kingpin.FatalIfError(err, "Root")

	ntfs.Debug(root.Offset())
	fmt.Println(root.DebugString())

	si, err := root.StandardInformation()
	kingpin.FatalIfError(err, "STANDARD_INFORMATION")
	fmt.Println(si.DebugString())

	for _, filename := range root.FileName() {
		fmt.Println(filename.DebugString())
	}

	fmt.Println("Nodes:")
	for _, node := range root.Dir() {
		fmt.Println(node.DebugString())
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "boot":
			doBoot()
		default:
			return false
		}
		return true
	})
}
