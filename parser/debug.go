package parser

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	debug_runtime "runtime/debug"

	"github.com/davecgh/go-spew/spew"
)

var (
	debug       = false
	LZNT1_debug = false

	NTFS_DEBUG *bool
)

func PrintStack() {
	debug_runtime.PrintStack()
}

func Debug(arg interface{}) {
	spew.Dump(arg)
}

type Debugger interface {
	DebugString() string
}

func DebugString(arg interface{}, indent string) string {
	debugger, ok := arg.(Debugger)
	if NTFS_DEBUG != nil && *NTFS_DEBUG && ok {
		lines := strings.Split(debugger.DebugString(), "\n")
		for idx, line := range lines {
			lines[idx] = indent + line
		}
		return strings.Join(lines, "\n")
	}

	return ""
}

func _DebugString(arg interface{}, indent string) string {
	debugger, ok := arg.(Debugger)
	if ok {
		lines := strings.Split(debugger.DebugString(), "\n")
		for idx, line := range lines {
			lines[idx] = indent + line
		}
		return strings.Join(lines, "\n")
	}

	return ""
}

func Printf(fmt_str string, args ...interface{}) {
	if NTFS_DEBUG != nil && *NTFS_DEBUG {
		fmt.Printf(fmt_str, args...)
	}
}

func LZNT1Printf(fmt_str string, args ...interface{}) {
	if LZNT1_debug {
		fmt.Printf(fmt_str, args...)
	}
}

// Turns on debugging programmatically
func SetDebug() {
	yes := true
	NTFS_DEBUG = &yes
}

func DebugPrint(fmt_str string, v ...interface{}) {
	if NTFS_DEBUG == nil {
		// os.Environ() seems very expensive in Go so we cache
		// it.
		for _, x := range os.Environ() {
			if strings.HasPrefix(x, "NTFS_DEBUG=1") {
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

// Debugging decompression
const (
	debugLZNT1 = false
)

func debugLZNT1Decompress(format string, args ...interface{}) {
	if debugLZNT1 {
		fmt.Printf(format, args...)
	}
}

func debugHexDump(buf []byte) string {
	if debugLZNT1 {
		return hex.Dump(buf)
	}
	return ""
}
