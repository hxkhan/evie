// Package fs is Evie's file system package. Every function blocks on the
// operating system, so each one releases the instance while it waits and
// other fibers keep running.
package fs

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the fs package
func Construct() *vm.Package {
	pkg := vm.NewPackage("fs")
	blocking := func(name string, fn *vm.GoFunc) {
		pkg.Native(name, fn).Blocking = true
	}

	blocking("readFile", vm.Func1E(os.ReadFile))
	blocking("readText", vm.Func1E(func(name string) (string, error) {
		data, err := os.ReadFile(name)
		return string(data), err
	}))
	blocking("writeFile", vm.Func2E(func(name string, content vm.Value) (vm.Value, error) {
		data, err := bytesOf(content)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, os.WriteFile(name, data, 0o644)
	}))
	blocking("appendFile", vm.Func2E(func(name string, content vm.Value) (vm.Value, error) {
		data, err := bytesOf(content)
		if err != nil {
			return vm.Value{}, err
		}
		f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return vm.Value{}, err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return vm.Value{}, err
		}
		return vm.Value{}, f.Close()
	}))
	blocking("exists", vm.Func1(func(name string) bool {
		_, err := os.Stat(name)
		return err == nil
	}))
	blocking("isDir", vm.Func1(func(name string) bool {
		info, err := os.Stat(name)
		return err == nil && info.IsDir()
	}))
	blocking("list", vm.Func1E(func(dir string) (*vm.Array, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		sort.Strings(names)
		values := make([]vm.Value, len(names))
		for i, name := range names {
			values[i] = vm.BoxString(name)
		}
		return vm.NewArray(values...), nil
	}))
	blocking("mkdir", vm.Func1E(func(dir string) (vm.Value, error) {
		return vm.Value{}, os.MkdirAll(dir, 0o755)
	}))
	blocking("remove", vm.Func1E(func(name string) (vm.Value, error) {
		return vm.Value{}, os.RemoveAll(name)
	}))
	blocking("rename", vm.Func2E(func(from, to string) (vm.Value, error) {
		return vm.Value{}, os.Rename(from, to)
	}))

	// path helpers touch no files and do not block
	pkg.Native("join", vm.Func2(func(a, b string) string { return filepath.ToSlash(filepath.Join(a, b)) }))
	pkg.Native("base", vm.Func1(filepath.Base))
	pkg.Native("dir", vm.Func1(func(name string) string { return filepath.ToSlash(filepath.Dir(name)) }))
	pkg.Native("ext", vm.Func1(filepath.Ext))
	return pkg
}

// bytesOf accepts a string or a buffer as file content
func bytesOf(content vm.Value) ([]byte, error) {
	if s, ok := content.AsString(); ok {
		return []byte(s), nil
	}
	if b, ok := content.AsBuffer(); ok {
		return b, nil
	}
	return nil, errors.New("expected a string or a buffer, got " + content.TypeOf())
}
