package main

import (
	"io"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	cat_command = app.Command(
		"cat", "List files.")

	cat_command_file_arg = cat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	cat_command_arg = cat_command.Arg(
		"path", "The path to extract to an MFT entry.",
	).Default("/").String()

	cat_command_output_file = cat_command.Flag(
		"out", "Write to this file",
	).String()
)

func doCAT() {
	file, err := getMFTEntry(*cat_command_arg, *cat_command_file_arg)
	kingpin.FatalIfError(err, "Can not open directory MFT entry.")

	for _, data := range file.Data() {
		if *cat_command_output_file != "" {
			filename := *cat_command_output_file

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
		case "cat":
			doCAT()
		default:
			return false
		}
		return true
	})
}
