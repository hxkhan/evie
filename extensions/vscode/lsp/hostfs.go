//go:build js && wasm

package main

import (
	"bytes"
	"io"
	"io/fs"
	"slices"
	"strings"
	"syscall/js"
	"time"
)

// hostFS is a workspace root as the JavaScript host sees it: open documents
// as the editor has them, everything else from disk. The host answers
// readFile(root, path) with a string or null, readDir(root, path) with an
// array of {name, dir} or null, and stat(root, path) with {dir} or null.
type hostFS struct {
	root string
	host js.Value
}

func (h hostFS) call(method, name string) js.Value {
	v := h.host.Call(method, h.root, name)
	if v.IsNull() || v.IsUndefined() {
		return js.Null()
	}
	return v
}

func (h hostFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}
	v := h.call("readFile", name)
	if v.IsNull() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return []byte(v.String()), nil
}

func (h hostFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	v := h.call("readDir", name)
	if v.IsNull() {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	entries := make([]fs.DirEntry, v.Length())
	for i := range entries {
		e := v.Index(i)
		entries[i] = info{name: e.Get("name").String(), dir: e.Get("dir").Bool()}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}

func (h hostFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	v := h.call("stat", name)
	if v.IsNull() {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	base := name[strings.LastIndex(name, "/")+1:]
	return info{name: base, dir: v.Get("dir").Bool()}, nil
}

func (h hostFS) Open(name string) (fs.File, error) {
	st, err := h.Stat(name)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		entries, err := h.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &dir{info: st.(info), entries: entries}, nil
	}
	data, err := h.ReadFile(name)
	if err != nil {
		return nil, err
	}
	i := st.(info)
	i.size = int64(len(data))
	return &file{info: i, Reader: bytes.NewReader(data)}, nil
}

// info describes a file or directory, and is also its directory entry
type info struct {
	name string
	dir  bool
	size int64
}

func (i info) Name() string               { return i.name }
func (i info) Size() int64                { return i.size }
func (i info) ModTime() time.Time         { return time.Time{} }
func (i info) IsDir() bool                { return i.dir }
func (i info) Sys() any                   { return nil }
func (i info) Type() fs.FileMode          { return i.Mode().Type() }
func (i info) Info() (fs.FileInfo, error) { return i, nil }
func (i info) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}

type file struct {
	info
	*bytes.Reader
}

func (f *file) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *file) Close() error               { return nil }

type dir struct {
	info
	entries []fs.DirEntry
	read    int
}

func (d *dir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (d *dir) Close() error               { return nil }
func (d *dir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *dir) ReadDir(n int) ([]fs.DirEntry, error) {
	rest := d.entries[d.read:]
	if n <= 0 {
		d.read = len(d.entries)
		return rest, nil
	}
	if len(rest) == 0 {
		return nil, io.EOF
	}
	n = min(n, len(rest))
	d.read += n
	return rest[:n], nil
}
