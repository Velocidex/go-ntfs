package ntfs

import (
	"fmt"
	"os"
	"strings"

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

func DebugPrint(fmt_str string, v ...interface{}) {
	for _, x := range os.Environ() {
		if strings.HasPrefix(x, "NTFS_DEBUG=") {
			fmt.Printf(fmt_str, v...)
			return
		}
	}
}
