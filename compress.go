package initramfs

import (
	"compress/bzip2"
	"compress/gzip"
	"io"
)

// A [CompressWriter] will compress anything written to it and write the
// compressed data to the given output.
//
// When compressing an initramfs archive using [Xz], review the [XZ compression
// options] to ensure compatibility.
//
// [XZ compression options]: https://www.kernel.org/doc/html/latest/staging/xz.html#notes-on-compression-options
type CompressWriter func(output io.Writer) (io.Writer, error)

// A [CompressWriter] using [compress/gzip.NewWriter].
func GzipWriter(w io.Writer) (io.Writer, error) { return gzip.NewWriter(w), nil }

// A [CompressReader] will decompress the given input.
type CompressReader func(input io.Reader) (io.Reader, error)

// Use the [Lookahead] token to select a suitable [CompressReader].
type CompressReaderMap map[Lookahead]CompressReader

// A global map of known compression readers.
//
// The default only includes compressors that exist within the standard library.
// See [go.pdmccormick.com/initramfs/examples] for sample implementations of
// existing packages.
var CompressReaders = CompressReaderMap{
	Gzip:  GzipReader,
	Bzip2: Bzip2Reader,
}

// A [CompressReader] using [compress/gzip.NewReader].
func GzipReader(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) }

// A [CompressReader] using [compress/bzip2.NewReader].
func Bzip2Reader(r io.Reader) (io.Reader, error) { return bzip2.NewReader(r), nil }
