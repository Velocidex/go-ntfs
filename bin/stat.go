package main

import (
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	stat_command = app.Command(
		"stat", "inspect the boot record.")

	stat_command_file_arg = stat_command.Arg(
		"file", "The image file to inspect",
	).Required().File()

	stat_command_arg = stat_command.Arg(
		"path", "The path to list or an STAT entry.",
	).Default("5").String()
)

func doSTAT() {
	root, err := getMFTEntry(*stat_command_arg, *stat_command_file_arg)
	kingpin.FatalIfError(err, "Can not open directory STAT entry.")

	fmt.Println(root.DebugString())
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
