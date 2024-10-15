package initramfs

import (
	"errors"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// Writer
type Writer struct {
	w io.Writer

	closed     bool
	compressed bool

	curW  io.Writer
	compW io.Writer

	mkdirs    map[string]struct{}
	nextInode uint32

	written       int64 // FIXME TODO: rename N
	fileRemaining int64

	dataAlignTo   int
	headerAlignTo int
}

var (
	ErrBadAlignment      = errors.New("initramfs: alignment must itself be a multiple of 4")
	ErrBadDataAlignment  = errors.New("initramfs: unable to align data as requested given the filename")
	ErrAlreadyCompressed = errors.New("initramfs: writer compression is already being applied")
)

func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:    w,
		curW: w,

		mkdirs: make(map[string]struct{}),
	}
}

func (iw *Writer) skipFileRemaining() (err error) {
	if n := iw.fileRemaining; n > 0 {
		err = iw.writePad(n)
		iw.fileRemaining = 0
	}
	return
}

func (iw *Writer) Write(buf []byte) (n int, err error) {
	if iw.closed {
		return 0, os.ErrClosed
	}

	if rem := iw.fileRemaining; rem == 0 {
		return 0, io.EOF
	} else if rem < int64(len(buf)) {
		n, err = iw.write(buf[:rem])
		if err == nil {
			err = io.EOF
		}
	} else {
		n, err = iw.write(buf)
	}

	if n > 0 {
		iw.fileRemaining -= int64(n)
	}

	return
}

// Reads file data from r and writes to the archive.
func (iw *Writer) ReadFrom(r io.Reader) (n int64, err error) {
	if iw.closed {
		return 0, os.ErrClosed
	}

	if rem := iw.fileRemaining; rem == 0 {
		return 0, io.EOF
	} else {
		n, err = io.CopyN(iw.curW, r, rem)
		if n > 0 {
			iw.written += n
			iw.fileRemaining -= n
		}
		return
	}
}

func (iw *Writer) write(p []byte) (int, error) {
	if iw.closed {
		return 0, os.ErrClosed
	}

	n, err := iw.curW.Write(p)
	if n > 0 {
		iw.written += int64(n)
	}
	return n, err
}

// Flush and close the writer.
func (iw *Writer) Close() error {
	if iw.closed {
		return os.ErrClosed
	}

	var (
		errs = [...]error{iw.Flush(), nil, nil}
		wrs  = [...]io.Writer{nil, iw.compW, iw.w}
	)

	for i, w := range wrs {
		if w != nil {
			if closer, ok := w.(io.Closer); ok {
				errs[i] = closer.Close()
			}
		}
	}

	iw.closed = true

	return errors.Join(errs[:]...)
}

// Flush any unwritten buffered output.
//
// The base [io.Writer] or [CompressWriter] must implement the [Flusher]
// interface for this to be effective.
func (iw *Writer) Flush() error {
	if iw.closed {
		return os.ErrClosed
	}

	var (
		errs = [...]error{nil, nil}
		wrs  = [...]io.Writer{iw.compW, iw.w}
	)

	for i, w := range wrs {
		if w != nil {
			if flusher, ok := w.(Flusher); ok {
				errs[i] = flusher.Flush()
			}
		}
	}

	return errors.Join(errs[:]...)
}

// Any writer that supports flushing its output.
type Flusher interface {
	Flush() error
}

// Before the start of a compressed stream within an archive, the output will be
// padded to match this alignment.
const StartCompressionAlignment = 512

// Switch the writer to a compressed output stream, according to the supplied
// [CompressWriter]. It is not possible to end a compressed stream other than by
// reaching the end of the file, so all remaining output from the writer will be
// compressed.
func (iw *Writer) StartCompression(c CompressWriter) error {
	if iw.closed {
		return os.ErrClosed
	}

	if iw.compressed {
		return ErrAlreadyCompressed
	}

	if err := iw.skipFileRemaining(); err != nil {
		return err
	}

	if err := iw.writeAlignment(StartCompressionAlignment); err != nil {
		return err
	}

	cw, err := c(iw.curW)
	if err != nil {
		return err
	}

	iw.curW = cw
	iw.compW = cw
	iw.compressed = true
	iw.written = 0

	return err
}

var zeroPadding [512]byte

// Write some number of 0 padding bytes.
func (iw *Writer) writePad(n int64) error {
	for n > 0 {
		var (
			k = min(n, int64(len(zeroPadding)))
			p = zeroPadding[:k]
		)
		m, err := iw.write(p)
		if err != nil {
			return err
		}
		n -= int64(m)
	}
	return nil
}

// Sets the output alignment for the start of the next header write. Value must
// itself be a multiple of 4.
//
// Only one of header or data alignment can be applied, and whichever is called
// last prior to calling [Writer.WriteHeader] will be applied. After every call
// to [Writer.WriteHeader] alignment is reset.
func (iw *Writer) SetHeaderAlignment(alignTo int) error {
	if alignTo%4 != 0 {
		return ErrBadAlignment
	}

	iw.dataAlignTo = 0
	iw.headerAlignTo = alignTo

	return nil
}

// Attempts to set the alignment of the file data by adjusting the amount of
// padding before the next header write. Value must itself be a multiple of 4.
//
// If the length of the header (110 bytes, see [HeaderSize]), plus the length of
// the NUL-terminated filename, is itself not a multiple of 4, the call to
// [Writer.WriteHeader] will return [ErrBadDataAlignment]. In the case of
// filename [MicrocodePath_GenuineIntel], this will work.
//
// Only one of header or data alignment can be applied, and whichever is called
// last prior to calling [Writer.WriteHeader] will be applied. After every call
// to [Writer.WriteHeader] alignment is reset.
func (iw *Writer) SetDataAlignment(alignTo int) error {
	if alignTo%4 != 0 {
		return ErrBadAlignment
	}

	iw.dataAlignTo = alignTo
	iw.headerAlignTo = 0

	return nil
}

func alignUp(n, to int64) int64 { return n + alignFill(n, to) }

func alignFill(n, to int64) int64 {
	if rem := n % to; rem > 0 {
		return to - rem
	}
	return 0
}

// Write sufficient padding such that the total number of output bytes written
// is a multiple of [alignTo].
func (iw *Writer) writeAlignment(alignTo int64) error {
	return iw.writePad(alignFill(iw.written, alignTo))
}

// Default permissions for directory entries
const DefaultMkdirPerm Mode = 0o700

func splitBytePrefixAll(s string, c byte) iter.Seq2[int, string] {
	return func(yield func(index int, prefix string) bool) {
		if !yield(0, ".") {
			return
		}

		for i := range s {
			if i > 0 && s[i] == c {
				if !yield(i, s[:i]) {
					return
				}
			}
		}

		if !yield(len(s), s) {
			return
		}
	}
}

func (iw *Writer) mkdir(path string, perm Mode) error {
	if path == "" {
		return nil
	}

	if _, ok := iw.mkdirs[path]; ok {
		return nil
	}

	var hdr = Header{
		Mode:     Mode_Dir | perm&Mode_PermsMask,
		Filename: path,
	}

	iw.mkdirs[path] = struct{}{}
	return iw.writeHeader(&hdr)
}

// Add a directory named path, along with any necessary parents, to the archive.
//
// The writer tracks which directories have already been added, and will skip
// any that already exist.
func (iw *Writer) MkdirAll(path string, perm Mode) error {
	if iw.closed {
		return os.ErrClosed
	}

	if perm == 0 {
		perm = DefaultMkdirPerm
	}

	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "."
	}

	if _, ok := iw.mkdirs[path]; ok {
		return nil
	}

	for _, prefix := range splitBytePrefixAll(path, '/') {
		if err := iw.mkdir(prefix, perm); err != nil {
			return err
		}
	}

	return nil
}

// Write the header in textual form, respecting output alignment requirements.
// The header will first be updated to ensure well-formedness:
//   - If Magic is blank, it will be given a default value of [Magic_070701]
//   - NumLinks will be minimum 1
//   - If Inode is 0 and this is not a trailer, an inode number will be assigned
//   - All leading slashes will be removed from the Filename
//   - FilenameSize will be set to the length of Filename plus 1
func (iw *Writer) WriteHeader(hdr *Header) error {
	if iw.closed {
		return os.ErrClosed
	}

	filename := strings.TrimPrefix(hdr.Filename, "/")
	if filename == "" {
		filename = "."
	}
	hdr.Filename = filename

	if hdr.Mode.Dir() {
		// Make note that this directory is being created
		iw.mkdirs[filename] = struct{}{}
	}

	if hdr.Trailer() {
		clear(iw.mkdirs)
	} else {
		// Ensure that all parent directories have been added
		dir := filepath.Dir(filename)

		if err := iw.MkdirAll(dir, 0); err != nil {
			return err
		}
	}

	return iw.writeHeader(hdr)
}

func (iw *Writer) writeHeader(hdr *Header) error {
	if err := iw.skipFileRemaining(); err != nil {
		return err
	}

	if hdr.Magic == "" {
		hdr.Magic = Magic_070701
	}

	if hdr.NumLinks == 0 {
		hdr.NumLinks = 1
	}

	if hdr.Inode == 0 && !hdr.Trailer() {
		hdr.Inode = iw.nextInode
	}

	iw.nextInode = max(iw.nextInode, hdr.Inode) + 1

	hdr.FilenameSize = uint32(len(hdr.Filename) + 1)

	if err := iw.writeAlignment(4); err != nil {
		return err
	}

	// As of this point, the output is guaranteed to be 4 byte aligned

	if alignTo := int64(iw.headerAlignTo); alignTo > 0 {
		if err := iw.writeAlignment(alignTo); err != nil {
			return err
		}
	} else if alignTo := int64(iw.dataAlignTo); alignTo > 0 {
		// How much padding do we need to achieve the desired data alignment
		// once this header and following alignment is applied?
		var fill = alignFill(iw.written+int64(hdr.Size()), alignTo)

		// If the fill length itself is not 4 byte aligned, then the post-header
		// alignment will throw off the data alignment.
		//
		// In this case, you would need to resort to the trick of writing an
		// empty file header with a filename of a specially crafted length.
		if fill%4 != 0 {
			return ErrBadDataAlignment
		}

		if err := iw.writePad(fill); err != nil {
			return err
		}
	}

	if n, err := hdr.WriteTo(iw.curW); err != nil {
		return err
	} else {
		iw.written += n
	}

	if err := iw.writeAlignment(4); err != nil {
		return err
	}

	iw.fileRemaining = int64(hdr.DataSize)

	// Any alignment resets after each call to WriteHeader
	iw.dataAlignTo = 0
	iw.headerAlignTo = 0

	return nil
}

// Write the end-of-archive sentinel trailer entry.
func (iw *Writer) WriteTrailer() error { return iw.WriteHeader(&trailerHeader) }
