package main

import (
	"encoding/json"
	"fmt"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	mft_command = app.Command(
		"mft", "Process a raw $MFT file.")

	mft_command_file_arg = mft_command.Arg(
		"file", "The $MFT file to process",
	).Required().File()
)

const (
	mft_entry_size int64 = 0x400
)

func doMFT() {
	reader, _ := parser.NewPagedReader(*mft_command_file_arg, 1024, 10000)
	st, err := (*mft_command_file_arg).Stat()
	kingpin.FatalIfError(err, "Can not open MFT file")

	for item := range parser.ParseMFTFile(reader, st.Size(), 0x1000, 0x400) {
		serialized, err := json.MarshalIndent(item, " ", " ")
		kingpin.FatalIfError(err, "Marshal")

		fmt.Println(string(serialized))
	}
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
