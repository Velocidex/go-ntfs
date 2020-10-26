package main

import (
	"os"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"www.velocidex.com/golang/go-ntfs/parser"
)

type CommandHandler func(command string) bool

var (
	app = kingpin.New("gontfs",
		"A tool for inspecting ntfs volumes.")

	verbose_flag = app.Flag(
		"verbose", "Show verbose information").Bool()

	command_handlers []CommandHandler
)

func main() {
	app.HelpFlag.Short('h')
	app.UsageTemplate(kingpin.CompactUsageTemplate)
	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	if *verbose_flag {
		parser.SetDebug()
	}

	for _, command_handler := range command_handlers {
		if command_handler(command) {
			break
		}
	}
}
