// Package strings is Evie's strings package: searching, splitting, trimming,
// padding and case. Arrays join with their own join method. Common checks such as len, trim, contains,
// startsWith and endsWith are also methods on every string.
package strings

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the strings package
func Construct() *vm.Package {
	pkg := vm.NewPackage("strings")

	pkg.Native("split", vm.Func2(func(s, sep string) *vm.Array { return boxAll(strings.Split(s, sep)) }))
	pkg.Native("fields", vm.Func1(func(s string) *vm.Array { return boxAll(strings.Fields(s)) }))
	pkg.Native("lines", vm.Func1(func(s string) *vm.Array {
		s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
		if s == "" {
			return vm.NewArray()
		}
		return boxAll(strings.Split(s, "\n"))
	}))

	pkg.Native("contains", vm.Func2(strings.Contains))
	pkg.Native("hasPrefix", vm.Func2(strings.HasPrefix))
	pkg.Native("hasSuffix", vm.Func2(strings.HasSuffix))
	pkg.Native("count", vm.Func2(strings.Count))
	pkg.Native("index", vm.Func2(func(s, sub string) int { return runeIndex(s, strings.Index(s, sub)) }))
	pkg.Native("lastIndex", vm.Func2(func(s, sub string) int { return runeIndex(s, strings.LastIndex(s, sub)) }))

	pkg.Native("replace", vm.Func3(func(s, old, new string) string { return strings.ReplaceAll(s, old, new) }))
	pkg.Native("replaceFirst", vm.Func3(func(s, old, new string) string { return strings.Replace(s, old, new, 1) }))
	pkg.Native("repeat", vm.Func2E(func(s string, n int) (string, error) {
		if n < 0 {
			return "", errors.New("repeat: the count must not be negative")
		}
		return strings.Repeat(s, n), nil
	}))
	pkg.Native("reverse", vm.Func1(func(s string) string {
		runes := []rune(s)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	}))

	pkg.Native("upper", vm.Func1(strings.ToUpper))
	pkg.Native("lower", vm.Func1(strings.ToLower))
	pkg.Native("trim", vm.Func1(strings.TrimSpace))
	pkg.Native("trimStart", vm.Func1(func(s string) string { return strings.TrimLeft(s, " \t\r\n") }))
	pkg.Native("trimEnd", vm.Func1(func(s string) string { return strings.TrimRight(s, " \t\r\n") }))
	pkg.Native("trimPrefix", vm.Func2(strings.TrimPrefix))
	pkg.Native("trimSuffix", vm.Func2(strings.TrimSuffix))
	pkg.Native("padStart", vm.Func3(func(s string, width int, pad string) string { return padding(s, width, pad) + s }))
	pkg.Native("padEnd", vm.Func3(func(s string, width int, pad string) string { return s + padding(s, width, pad) }))

	pkg.Native("chars", vm.Func1(func(s string) *vm.Array {
		chars := make([]vm.Value, 0, len(s))
		for _, r := range s {
			chars = append(chars, vm.BoxString(string(r)))
		}
		return vm.NewArray(chars...)
	}))
	pkg.Native("len", vm.Func1(utf8.RuneCountInString))
	pkg.Native("parseNumber", vm.Func1E(func(s string) (float64, error) {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0, fmt.Errorf("'%s' is not a number", s)
		}
		return f, nil
	}))
	return pkg
}

// boxAll turns Go strings into an Evie array of strings
func boxAll(parts []string) *vm.Array {
	values := make([]vm.Value, len(parts))
	for i, part := range parts {
		values[i] = vm.BoxString(part)
	}
	return vm.NewArray(values...)
}

// runeIndex converts a byte offset into s to a character offset; -1 stays -1
func runeIndex(s string, byteIndex int) int {
	if byteIndex < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:byteIndex])
}

// padding is the run of pad characters that widens s to width characters
func padding(s string, width int, pad string) string {
	missing := width - utf8.RuneCountInString(s)
	if missing <= 0 || pad == "" {
		return ""
	}
	padRunes := []rune(pad)
	out := make([]rune, missing)
	for i := range out {
		out[i] = padRunes[i%len(padRunes)]
	}
	return string(out)
}
