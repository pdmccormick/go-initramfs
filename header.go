package initramfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// Errors related to [Header].
var (
	ErrMalformedFilename = errors.New("initramfs: filename field is missing trailing 0")
	ErrBadHeaderMagic    = errors.New("initramfs: header contains a bad magic value")
)

// An invalid hexadecimal character was found at an offset relative to the start of a [Header].
type InvalidByteError int

func (offs *InvalidByteError) Error() string { return fmt.Sprintf("InvalidByteError(%d)", *offs) }

func invalidByteError(k int) error { var err = InvalidByteError(k); return &err }

// Magic identifiers for cpio archive member file headers.
const (
	Magic_070701 = `070701`
	Magic_070702 = `070702`
)

// The sentinel filename that indicates end-of-archive.
const TrailerFilename = "TRAILER!!!"

var trailerHeader = Header{
	Magic:        Magic_070701,
	NumLinks:     1,
	FilenameSize: uint32(len(TrailerFilename) + 1),
	Filename:     TrailerFilename,
}

// File mode and permission bits
type Mode uint32

func (m Mode) String() string {
	var s = [...]byte{'-', '-', '-', '-', '-', '-', '-', '-', '-', '-'}

	if m.Dir() {
		s[0] = 'd'
	}
	if m.Socket() {
		s[0] = 's'
	}
	if m.Symlink() {
		s[0] = 'l'
	}
	if m.BlockDevice() {
		s[0] = 'b'
	}

	if m.CharDevice() {
		s[0] = 'c'
	}
	if m.FIFO() {
		s[0] = 'p'
	}

	if (m & UserRead) != 0 {
		s[1] = 'r'
	}
	if (m & UserWrite) != 0 {
		s[2] = 'w'
	}
	if (m & UserExecute) != 0 {
		s[3] = 'x'
	}

	if (m & GroupRead) != 0 {
		s[4] = 'r'
	}
	if (m & GroupWrite) != 0 {
		s[5] = 'w'
	}
	if (m & GroupExecute) != 0 {
		s[6] = 'x'
	}

	if (m & OtherRead) != 0 {
		s[7] = 'r'
	}
	if (m & OtherWrite) != 0 {
		s[8] = 'w'
	}
	if (m & OtherExecute) != 0 {
		s[9] = 'x'
	}

	return string(s[:])
}

func (m Mode) FileType() Mode { return m & Mode_FileTypeMask }
func (m Mode) Perms() int     { return int(m & Mode_PermsMask) }

func (m Mode) Socket() bool      { return m.FileType() == Mode_Socket }
func (m Mode) Symlink() bool     { return m.FileType() == Mode_Symlink }
func (m Mode) File() bool        { return m.FileType() == Mode_File }
func (m Mode) BlockDevice() bool { return m.FileType() == Mode_BlockDevice }
func (m Mode) Dir() bool         { return m.FileType() == Mode_Dir }
func (m Mode) CharDevice() bool  { return m.FileType() == Mode_CharDevice }
func (m Mode) FIFO() bool        { return m.FileType() == Mode_FIFO }
func (m Mode) SUID() bool        { return m.FileType() == Mode_SUID }
func (m Mode) SGID() bool        { return m.FileType() == Mode_SGID }
func (m Mode) Sticky() bool      { return m.FileType() == Mode_Sticky }

func (m *Mode) SetFileType(ftype int) Mode {
	*m = (*m &^ Mode_FileTypeMask) | (Mode(ftype) & Mode_FileTypeMask)
	return *m
}

func (m *Mode) SetPerms(perms int) Mode {
	*m = (*m &^ Mode_PermsMask) | (Mode(perms) & Mode_PermsMask)
	return *m
}

func (m *Mode) SetBits(bits int) Mode {
	*m |= Mode(bits)
	return *m
}

func (m *Mode) ClearBits(bits int) Mode {
	*m &^= Mode(bits)
	return *m
}

func (m Mode) WithPerms(perms int) Mode { return m.SetPerms(perms) }

const (
	Mode_FileTypeMask Mode = 0o170_000
	Mode_Socket       Mode = 0o140_000 // File type for sockets.
	Mode_Symlink      Mode = 0o120_000 // File type for symbolic links (file data is link target).
	Mode_File         Mode = 0o100_000 // File type for regular files.
	Mode_BlockDevice  Mode = 0o060_000 // File type for block devices.
	Mode_Dir          Mode = 0o040_000 // File type for directories.
	Mode_CharDevice   Mode = 0o020_000 // File type for character devices.
	Mode_FIFO         Mode = 0o010_000 // File type for named pipes or FIFO's.
	Mode_SUID         Mode = 0o004_000 // SUID bit. See https://man7.org/linux/man-pages/man2/setfsuid.2.html#DESCRIPTION
	Mode_SGID         Mode = 0o002_000 // SGID bit. See https://man7.org/linux/man-pages/man2/setfsgid.2.html#DESCRIPTION
	Mode_Sticky       Mode = 0o001_000 // Sticky bit. See https://www.man7.org/linux/man-pages/man1/chmod.1.html#RESTRICTED_DELETION_FLAG_OR_STICKY_BIT
	Mode_PermsMask    Mode = 0o000_777 // Permission bits (read/write/execute for user, group and other). See https://man7.org/linux/man-pages/man1/chmod.1.html#DESCRIPTION

	UserRead     Mode = 0o400
	UserWrite    Mode = 0o200
	UserExecute  Mode = 0o100
	GroupRead    Mode = 0o040
	GroupWrite   Mode = 0o020
	GroupExecute Mode = 0o010
	OtherRead    Mode = 0o004
	OtherWrite   Mode = 0o002
	OtherExecute Mode = 0o001
)

// Header for a file member within a cpio archive.
type Header struct {
	HeaderOffset int64
	DataOffset   int64

	// Fixed length fields
	Magic        string    // Either `070701` or `070702`
	Inode        uint32    // File inode number
	Mode         Mode      // File mode and permission bits
	Uid          uint32    // File owner user id
	Gid          uint32    // File owner group id
	NumLinks     uint32    // Number of hard links
	Mtime        time.Time // Modification time (seconds since Unix epoch)
	DataSize     uint32    // Size of file data following the header
	Major        uint32    // Major part of file device number
	Minor        uint32    // Minor part of file device number
	RMajor       uint32    // Major part of device node reference
	RMinor       uint32    // Minor part of device node reference
	FilenameSize uint32    // Length of filename field (including trailing 0)
	Checksum     uint32    // Checksum of data field (if magic is `070702`, otherwise 0)

	// Variable length field
	Filename string
}

// Formats the header similarly to the long listing output of `ls -l`.
func (hdr *Header) String() string {
	return fmt.Sprintf("%s %4d  %4d %4d  %8d  %s  %s", hdr.Mode, hdr.NumLinks, hdr.Uid, hdr.Gid, hdr.DataSize, hdr.Mtime, hdr.Filename)
}

func (hdr *Header) Trailer() bool { return hdr.Filename == TrailerFilename }

// Read and convert the textual form of the header and filename fields.
//
// Returns an [InvalidByteError] if an invalid hexadecimal byte value is
// encountered. Returns [ErrMalformedFilename] if the filename field is missing
// a trailing 0.
func (hdr *Header) ReadFrom(r io.Reader) (n int64, err error) {
	var text rawTextHeader
	n0, err := text.ReadFrom(r)
	if err != nil {
		return n, err
	}

	n += int64(n0)

	if err := hdr.fromText(&text); err != nil {
		return n, err
	}

	var filename = make([]byte, hdr.FilenameSize)
	n1, err := io.ReadFull(r, filename)
	if err != nil {
		return n, err
	}

	n += int64(n1)

	if i := bytes.IndexByte(filename, 0); i == -1 {
		return n, ErrMalformedFilename
	} else {
		hdr.Filename = string(filename[:i])
	}

	return n, nil
}

// The length of the textual form of the header and filename fields.
func (hdr *Header) Size() int {
	return HeaderSize + len(hdr.Filename) + 1
}

// Return the textual form of the header and filename fields.
func (hdr *Header) Bytes() []byte {
	var (
		data = make([]byte, 0, hdr.Size())
		buf  = bytes.NewBuffer(data)
	)
	if _, err := hdr.WriteTo(buf); err != nil {
		return nil
	}
	return buf.Bytes()
}

// Write the textual form of the header and filename fields.
func (hdr *Header) WriteTo(w io.Writer) (n int64, err error) {
	var (
		filenameSize = len(hdr.Filename) + 1 // include trailing 0
		filename     = make([]byte, filenameSize)
	)

	hdr.FilenameSize = uint32(filenameSize)
	copy(filename[:], []byte(hdr.Filename))

	var text rawTextHeader
	if err := hdr.toText(&text); err != nil {
		return 0, err
	}

	n, err = text.writeTo(w)
	if err != nil {
		return
	}

	n1, err := w.Write(filename)
	if err != nil {
		return n, err
	}

	n += int64(n1)

	return n, nil
}

func (hdr *Header) fromText(text *rawTextHeader) error {
	var bin rawBinaryHeader

	if err := text.toBinary(&bin); err != nil {
		return err
	}

	var magic string
	switch bin.magicField() {
	case 0x070701:
		magic = Magic_070701
	case 0x070702:
		magic = Magic_070702
	default:
		return ErrBadHeaderMagic
	}

	*hdr = Header{
		Magic:        magic,
		Inode:        bin.field(0),
		Mode:         Mode(bin.field(1)),
		Uid:          bin.field(2),
		Gid:          bin.field(3),
		NumLinks:     bin.field(4),
		Mtime:        time.Unix(int64(bin.field(5)), 0),
		DataSize:     bin.field(6),
		Major:        bin.field(7),
		Minor:        bin.field(8),
		RMajor:       bin.field(9),
		RMinor:       bin.field(10),
		FilenameSize: bin.field(11),
		Checksum:     bin.field(12),
		// Filename is excluded from this conversion
	}

	return nil
}

func (hdr *Header) mtimeUnix() uint32 {
	if k := hdr.Mtime.Unix(); k < 0 {
		return 0
	} else {
		return uint32(k)
	}
}

func (hdr *Header) toText(text *rawTextHeader) error {
	var bin rawBinaryHeader

	bin.setField(0, hdr.Inode)
	bin.setField(1, uint32(hdr.Mode))
	bin.setField(2, hdr.Uid)
	bin.setField(3, hdr.Gid)
	bin.setField(4, hdr.NumLinks)
	bin.setField(5, hdr.mtimeUnix())
	bin.setField(6, hdr.DataSize)
	bin.setField(7, hdr.Major)
	bin.setField(8, hdr.Minor)
	bin.setField(9, hdr.RMajor)
	bin.setField(10, hdr.RMinor)
	bin.setField(11, hdr.FilenameSize)
	bin.setField(12, hdr.Checksum)
	// Filename is excluded from this conversion

	bin.toText(text)
	copy(text[0:6], hdr.Magic)

	return nil
}

// The size of a member file header within a cpio archive.
const HeaderSize = 110

// 6 bytes magic, 13 fields at 8 bytes each
var _ [HeaderSize]byte = [6 + 13*8]byte{}

// The raw hexadecimal characters of the fixed fields from the member file
// header.
type rawTextHeader [HeaderSize]byte

func (text *rawTextHeader) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.ReadFull(r, text[:])
	return int64(n), err
}

func (text *rawTextHeader) writeTo(w io.Writer) (int64, error) {
	n, err := w.Write(text[:])
	return int64(n), err
}

// Decode the hexadecimal characters into a binary form that is half as long.
// May return [InvalidByteError].
func (text *rawTextHeader) toBinary(bin *rawBinaryHeader) error {
	j := 0
	for i := range bin {
		hi, ok := hex2nibble(text[j])
		if !ok {
			return invalidByteError(j)
		}

		lo, ok := hex2nibble(text[j+1])
		if !ok {
			return invalidByteError(j + 1)
		}

		bin[i] = (hi << 4) | lo
		j += 2
	}
	return nil
}

// A cpio header after all its fixed fields have been converted from hex to
// binary.
type rawBinaryHeader [HeaderSize / 2]byte

func hex2nibble(h byte) (nibble byte, ok bool) {
	if '0' <= h && h <= '9' {
		return h - '0' + 0, true
	} else if 'a' <= h && h <= 'f' {
		return h - 'a' + 0xA, true
	} else if 'A' <= h && h <= 'F' {
		return h - 'A' + 0xA, true
	}
	return 0, false
}

func nibble2hex(nibble byte) byte {
	nibble = nibble & 0x0F

	if nibble <= 9 {
		return '0' + nibble
	} else if nibble >= 0xA {
		return 'A' + nibble - 0xA
	}
	return 0
}

func (bin *rawBinaryHeader) toText(text *rawTextHeader) {
	j := 0
	for i := range bin {
		var b = bin[i]
		text[j] = nibble2hex(b >> 4)
		text[j+1] = nibble2hex(b)
		j += 2
	}
}

const skipBinaryMagic = 3 // magic is 6 bytes of text => 3 bytes as binary

// Returns an unsigned 24-bit integer from the magic field.
func (bin *rawBinaryHeader) magicField() (v uint32) {
	v = uint32(bin[0])<<16 | uint32(bin[1])<<8 | uint32(bin[2])<<0
	return
}

// Returns an unsigned 32-bit field for the corresponding fixed field..
func (bin *rawBinaryHeader) field(i int) (v uint32) {
	var offs = skipBinaryMagic + 4*i
	v = uint32(bin[offs+0])<<24 | uint32(bin[offs+1])<<16 | uint32(bin[offs+2])<<8 | uint32(bin[offs+3])<<0
	return
}

func (bin *rawBinaryHeader) setField(i int, v uint32) {
	var offs = skipBinaryMagic + 4*i
	bin[offs+0] = byte((v >> 24) & 0xff)
	bin[offs+1] = byte((v >> 16) & 0xff)
	bin[offs+2] = byte((v >> 8) & 0xff)
	bin[offs+3] = byte((v >> 0) & 0xff)
}

// Compute the 32-bit unsigned sum of all the data bytes.
//
// This is the simple algorithm as used by the kernel for calculating the value
// of the [Header] Checksum field, as noted in the [buffer format]
// documentation.
//
// [buffer format]: https://www.kernel.org/doc/html/latest/driver-api/early-userspace/buffer-format.html
func ComputeChecksum(data []byte) (sum uint32) {
	for _, b := range data {
		sum += uint32(b)
	}
	return
}

// Computes the 32-bit unsigned sum of all the bytes from given reader. See
// [ComputeChecksum] for details.
func ReaderChecksum(r io.Reader) (sum uint32, err error) {
	var raw [512]byte

	for {
		n, err := r.Read(raw[:])
		if err != nil {
			if err == io.EOF {
				return sum, nil
			}

			return 0, err
		}

		sum += ComputeChecksum(raw[:n])
	}

	return
}
