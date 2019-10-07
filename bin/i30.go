package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

var (
	i30_command = app.Command(
		"i30", "Extract entries from an $I30 stream.")

	i30_command_file_arg = i30_command.Arg(
		"file", "The I30 stream to inspect.",
	).Required().OpenFile(os.O_RDONLY, os.FileMode(0666))

	i30_command_file_csv = i30_command.Flag(
		"csv", "Output in CSV.",
	).Bool()
)

func doI30() {
	reader, _ := parser.NewPagedReader(*i30_command_file_arg, 1024, 10000)
	ntfs := &parser.NTFSContext{
		Profile: parser.NewNTFSProfile()}

	stat, err := (*i30_command_file_arg).Stat()
	kingpin.FatalIfError(err, "stat")

	data := append([]*parser.FileInfo{},
		parser.ExtractI30ListFromStream(
			ntfs, reader, stat.Size())...)

	if *i30_command_file_csv {
		writer := csv.NewWriter(os.Stdout)
		defer writer.Flush()

		writer.Write([]string{"Name", "NameType", "Size", "AllocatedSize",
			"Mtime", "Atime", "Ctime", "Btime"})

		for _, info := range data {
			writer.Write([]string{
				info.Name,
				info.NameType,
				fmt.Sprintf("%v", info.Size),
				fmt.Sprintf("%v", info.AllocatedSize),
				fmt.Sprintf("%v", info.Mtime),
				fmt.Sprintf("%v", info.Atime),
				fmt.Sprintf("%v", info.Ctime),
				fmt.Sprintf("%v", info.Btime),
			})
		}

	} else {
		serialized, err := json.Marshal(data)
		kingpin.FatalIfError(err, "serialized")
		fmt.Printf("%v\n", string(serialized))
	}
}

func init() {
	command_handlers = append(command_handlers, func(command string) bool {
		switch command {
		case "i30":
			doI30()
		default:
			return false
		}
		return true
	})
}
