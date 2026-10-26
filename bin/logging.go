package main

import (
	"fmt"
	"time"
)

func Log(msg string, args ...interface{}) {
	if *verbose_flag {
		args = append(
			[]interface{}{time.Now().Format(time.RFC3339)}, args...)
		fmt.Printf("%s: "+msg, args)
	}
}
