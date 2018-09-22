package ntfs

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
)

var (
	debug       = false
	LZNT1_debug = false
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

func Printf(fmt_str string, args ...interface{}) {
	if debug {
		fmt.Printf(fmt_str, args...)
	}
}

func LZNT1Printf(fmt_str string, args ...interface{}) {
	if LZNT1_debug {
		fmt.Printf(fmt_str, args...)
	}
}
