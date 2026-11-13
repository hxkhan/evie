// Package fs is Evie's file system package. Every function blocks on the
// operating system, so each one releases the instance while it waits and
// other fibers keep running.
package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the fs package
func Construct() *vm.Package {
	pkg := vm.NewPackage("fs")
	blocking := func(name string, arity int, fn vm.NativeFn) {
		pkg.Func(name, arity, func(c vm.Call) (vm.Value, error) {
			if err := c.Context().Err(); err != nil {
				return vm.Value{}, context.Cause(c.Context())
			}
			return fn(c)
		}).Blocking = true
	}

	blocking("readFile", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxBuffer(data), nil
	})
	blocking("readText", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(string(data)), nil
	})
	blocking("writeFile", 2, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		data, err := bytesOf("writeFile", c.Arg(1))
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, os.WriteFile(name, data, 0o644)
	})
	blocking("appendFile", 2, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		data, err := bytesOf("appendFile", c.Arg(1))
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
	})
	blocking("exists", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		_, err = os.Stat(name)
		return vm.BoxBool(err == nil), nil
	})
	blocking("isDir", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		info, err := os.Stat(name)
		return vm.BoxBool(err == nil && info.IsDir()), nil
	})
	blocking("list", 1, func(c vm.Call) (vm.Value, error) {
		dir, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return vm.Value{}, err
		}
		values := make([]vm.Value, len(entries))
		for i, entry := range entries {
			values[i] = vm.BoxString(entry.Name())
		}
		return vm.BoxArray(vm.NewArray(values...)), nil
	})
	blocking("mkdir", 1, func(c vm.Call) (vm.Value, error) {
		dir, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, os.MkdirAll(dir, 0o755)
	})
	blocking("remove", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, os.RemoveAll(name)
	})
	blocking("rename", 2, func(c vm.Call) (vm.Value, error) {
		from, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		to, err := c.String(1)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, os.Rename(from, to)
	})

	// path helpers touch no files and do not block
	pkg.Variadic("join", 1, func(c vm.Call) (vm.Value, error) {
		parts := make([]string, c.Len())
		for i := range parts {
			s, err := c.String(i)
			if err != nil {
				return vm.Value{}, err
			}
			parts[i] = s
		}
		return vm.BoxString(filepath.ToSlash(filepath.Join(parts...))), nil
	})
	pkg.Native("base", vm.Func1(func(name string) string { return filepath.Base(filepath.FromSlash(name)) }))
	pkg.Native("dir", vm.Func1(func(name string) string { return filepath.ToSlash(filepath.Dir(filepath.FromSlash(name))) }))
	pkg.Native("ext", vm.Func1(func(name string) string { return filepath.Ext(filepath.FromSlash(name)) }))
	return pkg
}

// bytesOf accepts a string or a buffer as file content
func bytesOf(fn string, content vm.Value) ([]byte, error) {
	if s, ok := content.AsString(); ok {
		return []byte(s), nil
	}
	if b, ok := content.AsBuffer(); ok {
		return b, nil
	}
	return nil, fmt.Errorf("%s: argument 2 must be a string or a buffer, got %s", fn, content.TypeOf())
}
