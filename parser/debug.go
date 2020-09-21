package parser

import (
	"fmt"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

var (
	debug       = false
	LZNT1_debug = false

	NTFS_DEBUG *bool
)

func Debug(arg interface{}) {
	spew.Dump(arg)
}

type Debugger interface {
	DebugString() string
}

func DebugString(arg interface{}, indent string) string {
	debugger, ok := arg.(Debugger)
	if debug && ok {
		lines := strings.Split(debugger.DebugString(), "\n")
		for idx, line := range lines {
			lines[idx] = indent + line
		}
		return strings.Join(lines, "\n")
	}

	return ""
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
	if NTFS_DEBUG == nil {
		// os.Environ() seems very expensive in Go so we cache
		// it.
		for _, x := range os.Environ() {
			if strings.HasPrefix(x, "NTFS_DEBUG=") {
				value := true
				NTFS_DEBUG = &value
				break
			}
		}
	}

	if NTFS_DEBUG == nil {
		value := false
		NTFS_DEBUG = &value
	}

	if *NTFS_DEBUG {
		fmt.Printf(fmt_str, v...)
	}
}
