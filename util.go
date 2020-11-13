package ent

import (
	"fmt"
	"strings"
	"unsafe"
)

const intSize = int(32 << (^uint(0) >> 63)) // bits of int on target platform

type pointer = unsafe.Pointer

func logf(format string, v ...interface{}) {
	fmt.Printf("[ent] "+format+"\n", v...)
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// splitCommaSeparated returns a list of comma-separated values in the input string.
// E.g. ",foo, bar baz,lolcat " => ["", "foo", "bar baz", "lolcat"]
// Spaces around components are ignored.
func splitCommaSeparated(s string) []string {
	if len(s) == 0 {
		return nil
	}
	// "foo, bar,,baz" => ["foo"," bar","","baz"]
	tags := strings.Split(s, ",")
	for i, s := range tags {
		tags[i] = strings.TrimSpace(s)
	}
	return tags
}
