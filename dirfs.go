package initramfs

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Like [os.DirFS], but does not follow symbolic links. Reading from a symbolic
// link file returns the target of the link. Files returned by Open also
// implement [ReadLinkFile].
func LinkAwareDirFS(dir string) fs.FS {
	return linkAwareDirFS(dir)
}

type linkAwareDirFS string

var (
	_ fs.FS         = (linkAwareDirFS)("")
	_ fs.ReadFileFS = (linkAwareDirFS)("")
	_ fs.ReadDirFS  = (linkAwareDirFS)("")
	_ fs.GlobFS     = (linkAwareDirFS)("")
	_ fs.StatFS     = (linkAwareDirFS)("")
)

func (d linkAwareDirFS) osDirFS() fs.FS          { return os.DirFS(string(d)) }
func (d linkAwareDirFS) path(name string) string { return filepath.Join(string(d), name) }

func (d linkAwareDirFS) lstat(name string) (path string, fi fs.FileInfo, isSymlink bool, err error) {
	path = d.path(name)
	fi, err = os.Lstat(path)
	if err == nil && fi != nil {
		isSymlink = fi.Mode().Type() == fs.ModeSymlink
	}
	return
}

func (d linkAwareDirFS) Stat(name string) (fi fs.FileInfo, err error) {
	_, fi, _, err = d.lstat(name)
	return
}

func (d linkAwareDirFS) Glob(pattern string) ([]string, error) { return fs.Glob(d.osDirFS(), pattern) }

func (d linkAwareDirFS) Open(name string) (fs.File, error) {
	path, fi, isSymlink, err := d.lstat(name)
	if err != nil {
		return nil, err
	}

	if isSymlink {
		return d.openLink(path, fi)
	}

	f, err := d.osDirFS().Open(strings.TrimPrefix(name, "/"))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (d linkAwareDirFS) openLink(path string, info fs.FileInfo) (*linkFile, error) {
	target, err := os.Readlink(path)
	if err != nil {
		return nil, err
	}

	lf := linkFile{path, info, target, []byte(target), false}
	return &lf, nil
}

func (d linkAwareDirFS) ReadFile(name string) ([]byte, error) {
	path, fi, isSymlink, err := d.lstat(name)
	if err != nil {
		return nil, err
	}

	if isSymlink {
		lf, err := d.openLink(path, fi)
		if err != nil {
			return nil, err
		}

		return lf.buf, nil
	}

	return fs.ReadFile(d.osDirFS(), strings.TrimPrefix(name, "/"))
}

func (d linkAwareDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	ents, err := os.ReadDir(d.path(name))
	if err != nil {
		return nil, err
	}

	for i, ent := range ents {
		_, fi, isSymlink, err := d.lstat(ent.Name())
		if err != nil || !isSymlink {
			continue
		}

		ents[i] = &linkDirEntry{ent, fi}
	}

	return ents, nil
}

type linkDirEntry struct {
	ent  fs.DirEntry
	info fs.FileInfo
}

var _ fs.DirEntry = (*linkDirEntry)(nil)

func (l *linkDirEntry) Info() (fs.FileInfo, error) { return l.info, nil }
func (l *linkDirEntry) IsDir() bool                { return false }
func (l *linkDirEntry) Name() string               { return l.ent.Name() }
func (l *linkDirEntry) Type() fs.FileMode          { return fs.ModeSymlink }

type ReadLinkFile interface {
	ReadLink() (string, error)
}

type linkFile struct {
	path   string
	info   fs.FileInfo
	target string
	buf    []byte
	closed bool
}

var (
	_ fs.File      = (*linkFile)(nil)
	_ ReadLinkFile = (*linkFile)(nil)
)

func (lf *linkFile) Close() error {
	if lf.closed {
		return os.ErrClosed
	}
	lf.buf = nil
	lf.closed = true
	return nil
}

func (lf *linkFile) Read(p []byte) (int, error) {
	if lf.closed {
		return 0, os.ErrClosed
	}

	if len(lf.buf) == 0 {
		return 0, io.EOF
	}

	n := copy(p, lf.buf)
	lf.buf = lf.buf[n:]

	if len(lf.buf) == 0 {
		lf.buf = nil
	}

	return n, nil
}

func (lf *linkFile) Stat() (fs.FileInfo, error) { return lf.info, nil }
func (lf *linkFile) ReadLink() (string, error)  { return lf.target, nil }
