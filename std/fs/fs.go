// Package fs is Evie's file system package. Every function blocks on the
// operating system, so each one releases the instance while it waits and
// other fibers keep running.
package fs

import (
	"context"
	"errors"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the fs package on the operating system's file system
func Construct() *vm.Package {
	return build(osFiles{})
}

// ConstructFS builds an fs package confined to fsys: scripts can read what fsys
// holds and nothing else, and every function that writes fails. Paths are
// slash-separated and relative to the root of fsys.
func ConstructFS(fsys iofs.FS) *vm.Package {
	return build(fsFiles{fsys})
}

// files is where the package reads and writes
type files interface {
	open(name string) (iofs.File, error)
	stat(name string) (iofs.FileInfo, error)
	readDir(name string) ([]iofs.DirEntry, error)
	writeFile(name string, data []byte) error
	appendFile(name string, data []byte) error
	mkdir(name string) error
	remove(name string) error
	rename(from, to string) error
}

// checked reports what goes wrong the same way on every system, so a script
// can rely on it: a missing file is a "notFound" error that names the file,
// whether Evie runs on Linux, Windows or in a browser
type checked struct{ files }

func (c checked) open(name string) (iofs.File, error) {
	f, err := c.files.open(name)
	return f, failed(name, err)
}
func (c checked) stat(name string) (iofs.FileInfo, error) {
	info, err := c.files.stat(name)
	return info, failed(name, err)
}
func (c checked) readDir(name string) ([]iofs.DirEntry, error) {
	entries, err := c.files.readDir(name)
	return entries, failed(name, err)
}
func (c checked) writeFile(name string, data []byte) error {
	return failed(name, c.files.writeFile(name, data))
}
func (c checked) appendFile(name string, data []byte) error {
	return failed(name, c.files.appendFile(name, data))
}
func (c checked) mkdir(name string) error  { return failed(name, c.files.mkdir(name)) }
func (c checked) remove(name string) error { return failed(name, c.files.remove(name)) }
func (c checked) rename(from, to string) error {
	err := c.files.rename(from, to)
	if errors.Is(err, iofs.ErrExist) {
		return failed(to, err)
	}
	return failed(from, err)
}

// failed turns what the operating system or a file system returned for the
// file name into an error with a kind, a message that names the file, and the
// file as the field path. The original error stays its cause for the host.
func failed(name string, err error) error {
	if err == nil {
		return nil
	}
	if exc, ok := err.(*vm.Error); ok {
		return exc
	}
	var e *vm.Error
	switch {
	case errors.Is(err, errReadOnly):
		e = vm.Errorf(vm.KindPermission, "cannot change '%s': the file system of this engine is read-only", name)
	case errors.Is(err, errOutside):
		e = vm.Errorf(vm.KindPermission, "'%s' is outside the file system of this engine", name)
	case errors.Is(err, iofs.ErrNotExist):
		e = vm.Errorf(vm.KindNotFound, "'%s' does not exist", name)
	case errors.Is(err, iofs.ErrExist):
		e = vm.Errorf(vm.KindExists, "'%s' already exists", name)
	case errors.Is(err, iofs.ErrPermission):
		e = vm.Errorf(vm.KindPermission, "no permission to access '%s'", name)
	default:
		e = vm.Errorf(vm.KindIO, "%s", err)
	}
	e.Cause = err
	return e.With("path", vm.BoxString(name))
}

func build(inner files) *vm.Package {
	files := checked{inner}
	pkg := vm.NewPackage("fs")
	blocking := func(name string, arity int, fn vm.NativeFn) {
		pkg.Func(name, arity, func(c vm.Call) (vm.Value, error) {
			if err := c.Context().Err(); err != nil {
				return vm.Value{}, context.Cause(c.Context())
			}
			return fn(c)
		}).Blocking = true
	}
	// onPath binds a function of one path argument
	onPath := func(name string, fn func(path string) (vm.Value, error)) {
		blocking(name, 1, func(c vm.Call) (vm.Value, error) {
			p, err := c.String(0)
			if err != nil {
				return vm.Value{}, err
			}
			return fn(p)
		})
	}
	// withContent binds a function of a path and a string or buffer
	withContent := func(name string, fn func(path string, data []byte) error) {
		blocking(name, 2, func(c vm.Call) (vm.Value, error) {
			p, err := c.String(0)
			if err != nil {
				return vm.Value{}, err
			}
			data, err := bytesOf(name, c.Arg(1))
			if err != nil {
				return vm.Value{}, err
			}
			return vm.Value{}, fn(p, data)
		})
	}

	blocking("readFile", 1, func(c vm.Call) (vm.Value, error) {
		data, err := readFile(c, files)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxBuffer(data), nil
	})
	blocking("readText", 1, func(c vm.Call) (vm.Value, error) {
		data, err := readFile(c, files)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxString(string(data)), nil
	})
	withContent("writeFile", files.writeFile)
	withContent("appendFile", files.appendFile)
	onPath("exists", func(p string) (vm.Value, error) {
		_, err := files.stat(p)
		return vm.BoxBool(err == nil), nil
	})
	onPath("isDir", func(p string) (vm.Value, error) {
		info, err := files.stat(p)
		return vm.BoxBool(err == nil && info.IsDir()), nil
	})
	onPath("list", func(p string) (vm.Value, error) {
		entries, err := files.readDir(p)
		if err != nil {
			return vm.Value{}, err
		}
		values := make([]vm.Value, len(entries))
		for i, entry := range entries {
			values[i] = vm.BoxString(entry.Name())
		}
		return vm.BoxArray(vm.NewArray(values...)), nil
	})
	onPath("mkdir", func(p string) (vm.Value, error) { return vm.Value{}, files.mkdir(p) })
	onPath("remove", func(p string) (vm.Value, error) { return vm.Value{}, files.remove(p) })
	blocking("rename", 2, func(c vm.Call) (vm.Value, error) {
		from, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		to, err := c.String(1)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.Value{}, files.rename(from, to)
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
	// what tools such as editors show for each member
	pkg.Document("readFile", "readFile(path)", "The file's contents as a buffer.")
	pkg.Document("readText", "readText(path)", "The file's contents as a string.")
	pkg.Document("writeFile", "writeFile(path, content)", "Writes a string or buffer to the file, replacing it.")
	pkg.Document("appendFile", "appendFile(path, content)", "Adds a string or buffer to the end of the file.")
	pkg.Document("exists", "exists(path)", "Whether the path exists.")
	pkg.Document("isDir", "isDir(path)", "Whether the path is a directory.")
	pkg.Document("list", "list(dir)", "The sorted names in a directory.")
	pkg.Document("mkdir", "mkdir(dir)", "Creates a directory and its parents.")
	pkg.Document("remove", "remove(path)", "Removes a file, or a directory and everything in it.")
	pkg.Document("rename", "rename(from, to)", "Moves a file or directory.")
	pkg.Document("join", "join(...parts)", "Joins path parts with '/'; does not touch the disk.")
	pkg.Document("base", "base(path)", "The last element of a path; does not touch the disk.")
	pkg.Document("dir", "dir(path)", "A path without its last element; does not touch the disk.")
	pkg.Document("ext", "ext(path)", "A path's extension, such as \".ev\"; does not touch the disk.")
	return pkg
}

// osFiles is the operating system's file system, relative to the working directory
type osFiles struct{}

func (osFiles) open(name string) (iofs.File, error)          { return os.Open(name) }
func (osFiles) stat(name string) (iofs.FileInfo, error)      { return os.Stat(name) }
func (osFiles) readDir(name string) ([]iofs.DirEntry, error) { return os.ReadDir(name) }
func (osFiles) writeFile(name string, data []byte) error     { return os.WriteFile(name, data, 0o644) }
func (osFiles) mkdir(name string) error                      { return os.MkdirAll(name, 0o755) }
func (osFiles) remove(name string) error                     { return os.RemoveAll(name) }
func (osFiles) rename(from, to string) error                 { return os.Rename(from, to) }

func (osFiles) appendFile(name string, data []byte) error {
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fsFiles is a read-only view of a host-supplied file system
type fsFiles struct{ fsys iofs.FS }

// errReadOnly is what every write gets from an engine confined to a file
// system, and errOutside what a path that leaves it gets
var (
	errReadOnly = errors.New("the file system of this engine is read-only")
	errOutside  = errors.New("outside the file system of this engine")
)

// clean turns a script's path into a name fsys accepts: slash-separated, with
// no leading slash and no '..' that would step outside the root
func clean(name string) (string, error) {
	p := path.Clean(filepath.ToSlash(name))
	if !iofs.ValidPath(p) {
		return "", errOutside
	}
	return p, nil
}

func (f fsFiles) open(name string) (iofs.File, error) {
	p, err := clean(name)
	if err != nil {
		return nil, err
	}
	return f.fsys.Open(p)
}

func (f fsFiles) stat(name string) (iofs.FileInfo, error) {
	p, err := clean(name)
	if err != nil {
		return nil, err
	}
	return iofs.Stat(f.fsys, p)
}

func (f fsFiles) readDir(name string) ([]iofs.DirEntry, error) {
	p, err := clean(name)
	if err != nil {
		return nil, err
	}
	return iofs.ReadDir(f.fsys, p)
}

func (fsFiles) writeFile(string, []byte) error  { return errReadOnly }
func (fsFiles) appendFile(string, []byte) error { return errReadOnly }
func (fsFiles) mkdir(string) error              { return errReadOnly }
func (fsFiles) remove(string) error             { return errReadOnly }
func (fsFiles) rename(string, string) error     { return errReadOnly }

// maxReadBytes caps what one read takes into memory, so that reading a device
// or a pipe that never ends, such as /dev/zero, fails instead of exhausting it
const maxReadBytes = 256 << 20

// readFile reads the file named by argument 0 in chunks, giving up when the
// call has to stop or the file outgrows maxReadBytes
func readFile(c vm.Call, files files) ([]byte, error) {
	name, err := c.String(0)
	if err != nil {
		return nil, err
	}
	f, err := files.open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data []byte
	chunk := make([]byte, 1<<20)
	for {
		if err := c.Context().Err(); err != nil {
			return nil, context.Cause(c.Context())
		}
		n, err := f.Read(chunk)
		if len(data)+n > maxReadBytes {
			return nil, vm.Errorf(vm.KindIO, "'%s' is larger than %d MB, the most a read takes", name, maxReadBytes>>20).With("path", vm.BoxString(name))
		}
		data = append(data, chunk[:n]...)
		if errors.Is(err, io.EOF) {
			return data, nil
		}
		if err != nil {
			return nil, failed(name, err)
		}
	}
}

// bytesOf accepts a string or a buffer as file content
func bytesOf(fn string, content vm.Value) ([]byte, error) {
	if s, ok := content.AsString(); ok {
		return []byte(s), nil
	}
	if b, ok := content.AsBuffer(); ok {
		return b, nil
	}
	return nil, vm.Errorf(vm.KindType, "%s: argument 2 must be a string or a buffer, got %s", fn, content.TypeOf())
}
