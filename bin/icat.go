package main

import (
	"io"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	ntfs "www.velocidex.com/golang/go-ntfs"
)

var (
	icat_command = app.Command(
		"icat", "List files.")

	icat_command_file_arg = icat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	icat_command_arg = icat_command.Arg(
		"MFT", "The MFT ID to list (5 is the root).",
	).Default("5").Int64()

	icat_command_output_file = icat_command.Flag(
		"out", "Write to this file",
	).String()
)

func doICAT() {
	profile, err := ntfs.GetProfile()
	kingpin.FatalIfError(err, "Profile")

	boot, err := ntfs.NewBootRecord(profile, *icat_command_file_arg, 0)
	kingpin.FatalIfError(err, "Boot record")

	mft, err := boot.MFT()
	kingpin.FatalIfError(err, "MFT")

	file, err := mft.MFTEntry(*icat_command_arg)
	kingpin.FatalIfError(err, "Root")

	for _, data := range file.Data() {
		if *icat_command_output_file != "" {
			filename := *icat_command_output_file

			if data.Name() != "" {
				filename += "_" + data.Name()
			}

			fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, os.FileMode(0666))
			kingpin.FatalIfError(err, "Open file")
			defer fd.Close()

			_, err = io.Copy(
				fd, io.NewSectionReader(
					data.Data(), 0, data.Size()))
			kingpin.FatalIfError(err, "Reading file")

		} else {
			_, err = io.Copy(
				os.Stdout, io.NewSectionReader(
					data.Data(), 0, data.Size()))
			kingpin.FatalIfError(err, "Reading file")
		}
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "icat":
			doICAT()
		default:
			return false
		}
		return true
	})
}
