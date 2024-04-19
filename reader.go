package initramfs

import (
	"bufio"
	"errors"
	"io"
	"iter"
)

type Reader struct {
	r     io.Reader
	br    *bufio.Reader
	nread int64
	fileR io.LimitedReader
}

var (
	_ io.Reader   = (*Reader)(nil)
	_ io.WriterTo = (*Reader)(nil)
)

func NewReader(r io.Reader) *Reader {
	var br = bufio.NewReader(r)
	return &Reader{
		r:     r,
		br:    br,
		fileR: io.LimitedReader{R: br},
	}
}

// Consumes input looking for the next file entry. Returns
// [ErrCompressedContentAhead] if the start of compress data has been detected.
//
// Check for compressed data by calling [Reader.ContinueCompressed].
func (r *Reader) Next() (*Header, error) {
	var hdr Header
	if err := r.next(&hdr); err != nil {
		return nil, err
	}
	return &hdr, nil
}

// Reads file data up to the length indicated by [Header.DataSize].
func (r *Reader) Read(buf []byte) (int, error) { return r.fileR.Read(buf) }

// Copy all remaining current file data to the writer.
func (r *Reader) WriteTo(w io.Writer) (n int64, err error) {
	if rem := r.fileR.N; rem == 0 {
		return 0, io.EOF
	} else {
		n, err = io.CopyN(w, r.br, rem)
		r.fileR.N -= n
		return
	}
}

// Provides a sequence iterator that is equivalent to calling [Reader.Next]
// until EOF.
func (r *Reader) All() iter.Seq2[int, Header] {
	return func(yield func(index int, hdr Header) bool) {
		for i := 0; ; i++ {
			var hdr Header
			if err := r.next(&hdr); err != nil {
				return
			}

			if !yield(i, hdr) {
				return
			}
		}
	}
}

func (r *Reader) skipUnreadFile() (err error) {
	if n := r.fileR.N; n > 0 {
		r.fileR.N = 0
		_, err = r.br.Discard(int(n))
	}
	return
}

func (r *Reader) advanceToNextHeader() error {
	if err := r.skipUnreadFile(); err != nil {
		return err
	}

Advance:
	for {
		peek, err := PeekLookahead(r.br)
		if err != nil {
			return err
		}

		if peek.Compression() {
			return ErrCompressedContentAhead
		}

		switch peek {
		case EOF:
			return io.EOF

		case Padding:
			if err := r.discardPadding(); err != nil {
				return err
			}
			continue Advance

		case CpioFile:
			break Advance

		default:
			return errors.New("initramfs: unknown error")
		}
	}

	return nil
}

func (r *Reader) next(hdr *Header) error {
	if err := r.advanceToNextHeader(); err != nil {
		return err
	}

	var headerOffset = r.nread

	n, err := hdr.ReadFrom(r.br)
	if n > 0 {
		r.nread += n
	}

	hdr.HeaderOffset = headerOffset

	if err != nil {
		return err
	}

	if err := r.discardAlign(4); err != nil {
		return err
	}

	hdr.DataOffset = r.nread
	r.fileR.N = int64(hdr.DataSize)

	// Assume file has already been read for the purposes of tracking current read position
	r.nread += r.fileR.N

	return nil
}

var ErrCompressedContentAhead = errors.New("initramfs: compressed content ahead")

var ErrNoCompressReader = errors.New("initramfs: no suitable CompressReader found")

// Attempt to continue reader into the start of a compressed data stream.
//
// Returns [ErrNoCompressReader] if the [CompressReaderMap] does not contain a
// suitable reader for the encountered compression type.
func (r *Reader) ContinueCompressed(compressReaders CompressReaderMap) (isCompressed bool, compressType Lookahead, err error) {
	err = r.skipUnreadFile()
	if err != nil {
		return
	}

	err = r.discardPadding()
	if err != nil {
		return
	}

	compressType, err = PeekLookahead(r.br)
	if err != nil {
		return
	}

	if compressType == EOF {
		err = io.EOF
		return
	}

	if !compressType.Compression() {
		return
	}

	isCompressed = true

	if compressReaders == nil {
		compressReaders = CompressReaders
	}

	dec, ok := compressReaders[compressType]
	if !ok {
		err = ErrNoCompressReader
		return
	}

	var dr io.Reader
	dr, err = dec(r.br)
	if err != nil {
		return
	}

	r.r = dr
	r.br = bufio.NewReader(dr)
	r.fileR.R = r.br
	r.nread = 0

	return
}

func (r *Reader) discard(n int64) error {
	if n > 0 {
		if _, err := r.br.Discard(int(n)); err != nil {
			return err
		}
		r.nread += n
	}
	return nil
}

func (r *Reader) discardPadding() error {
	for {
		const N = 64

		peek, err := r.br.Peek(N)
		if err != nil {
			return err
		}

		var n int64
		for i, b := range peek {
			if b == 0 {
				n = int64(i) + 1
			} else {
				break
			}
		}

		if n > 0 {
			r.discard(n)
		}

		if n != N {
			break
		}
	}

	return nil
}

func (r *Reader) discardAlign(n int) error {
	var n64 = int64(n)
	if rem := r.nread % n64; rem > 0 {
		return r.discard(n64 - rem)
	}
	return nil
}
