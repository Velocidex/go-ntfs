package ntfs

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
)

var (
	debug = false
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

func Printf(fmt_str string, args ...interface{}) {
	if debug {
		fmt.Printf(fmt_str, args...)
	}
}
