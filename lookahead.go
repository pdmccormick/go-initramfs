package initramfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

// Identify what kind of data comes next in a stream by looking ahead a few
// bytes and identifying magic values.
//
// Recognizes the difference between cpio archive member file headers, zero
// padding and various compression schemes. See the RD_ and
// INITRAMFS_COMPRESSION_ config options in [Linux kernel usr/Kconfig].
//
// [Linux kernel usr/Kconfig]: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/usr/Kconfig
type Lookahead int

const (
	UnknownLookahead Lookahead = iota
	EOF                        // End of file
	Padding                    // Zero padding
	CpioFile                   // Start of cpio archive member file header
	Gzip                       // Start of Gzip compressed data
	Bzip2                      // Start of Bzip2 compressed data
	Lzma                       // Start of LZMA compressed data
	Xz                         // Start of XZ compressed data
	Lzo                        // Start of LZO compressed data
	Lz4                        // Start of LZ4 compressed data
	Zstd                       // Start of Zstd compressed data
)

var (
	magic_070701 = []byte(Magic_070701)
	magic_070702 = []byte(Magic_070702)
)

// Uses [bufio.Reader.Peek] to determine what kind of data follows. Does not
// consume the input. Only returns non-EOF errors.
func PeekLookahead(br *bufio.Reader) (la Lookahead, err error) {
	peek, err := br.Peek(2)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return EOF, nil
		}

		return UnknownLookahead, err
	}

	if peek[0] == 0 {
		return Padding, nil
	}

	var m = Magic(peek[0])<<8 | Magic(peek[1])
	switch m {
	case CpioFileMagic:
		if peek, err = br.Peek(6); err != nil {
			return UnknownLookahead, err
		} else if bytes.Equal(peek, magic_070701) || bytes.Equal(peek, magic_070702) {
			return CpioFile, nil
		}

	case GzipMagic1:
		return Gzip, nil
	case GzipMagic2:
		return Gzip, nil
	case Bzip2Magic:
		return Bzip2, nil
	case LzmaMagic:
		return Lzma, nil
	case XzMagic:
		return Xz, nil
	case LzoMagic:
		return Lzo, nil
	case Lz4Magic:
		return Lz4, nil
	case ZstdMagic:
		return Zstd, nil
	}

	return UnknownLookahead, nil
}

// Returns true if and only if the lookahead indicates the start of compressed data.
func (la Lookahead) Compression() bool {
	switch la {
	case Gzip,
		Bzip2,
		Lzma,
		Xz,
		Lzo,
		Lz4,
		Zstd:
		return true
	default:
		return false
	}
}

// If the end of file was reached when looking ahead.
func (la Lookahead) EOF() bool { return la == EOF }

func (la Lookahead) String() string {
	switch la {
	case UnknownLookahead:
		return "unknown"
	case EOF:
		return "EOF"
	case Padding:
		return "padding"
	case CpioFile:
		return "cpiofile"
	case Gzip:
		return "gzip"
	case Bzip2:
		return "bzip2"
	case Lzma:
		return "lzma"
	case Xz:
		return "xz"
	case Lzo:
		return "lzo"
	case Lz4:
		return "lz4"
	case Zstd:
		return "zstd"
	default:
		return fmt.Sprintf("0x%x", int(la))
	}
}

// Magic byte values used to identify the start of various types of compressed
// data streams.
//
// These match what the kernel uses, see [Linux kernel lib/decompress.c].
//
// [Linux kernel lib/decompress.c]: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/lib/decompress.c
type Magic uint16

const (
	CpioFileMagic Magic = 0x30_37 // A cpio archive member file header starts with "07" (either "070701" or "070702")
	GzipMagic1    Magic = 0x1F_8B
	GzipMagic2    Magic = 0x1F_9E
	Bzip2Magic    Magic = 0x42_5A
	LzmaMagic     Magic = 0x5D_00
	XzMagic       Magic = 0xFD_37
	LzoMagic      Magic = 0x89_4C
	Lz4Magic      Magic = 0x02_21
	ZstdMagic     Magic = 0x28_B5
)

// Determine if the provided bytes are a recognized magic value.
func SniffMagic(peek [2]byte) (m Magic, ok bool) {
	m = Magic(peek[0])<<8 | Magic(peek[1])
	switch m {
	case CpioFileMagic,
		GzipMagic1,
		GzipMagic2,
		Bzip2Magic,
		LzmaMagic,
		XzMagic,
		LzoMagic,
		Lz4Magic,
		ZstdMagic:
		return m, true
	default:
		return
	}
}

// Returns true if the bytes match the magic value.
func (m Magic) Match(a [2]byte) bool { return m == Magic(a[0])<<8|Magic(a[1]) }

func (m Magic) MatchBytes(p []byte) bool {
	var a [2]byte
	copy(a[:], p)
	return m.Match(a)
}
