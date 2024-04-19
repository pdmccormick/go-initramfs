package examples

import (
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"go.pdmccormick.com/initramfs"
)

// An Xz [go.pdmccormick.com/initramfs.CompressReader] using the [github.com/ulikunitz/xz] package.
func XzReader(r io.Reader) (io.Reader, error) { return xz.NewReader(r) }

// An Zstd [go.pdmccormick.com/initramfs.CompressReader] using the [github.com/klauspost/compress/zstd]
func ZstdReader(r io.Reader) (io.Reader, error) { return zstd.NewReader(r) }

// Adds [XzReader] and [ZstdReader] to the global [go.pdmccormick.com/initramfs.CompressReaders] map.
func SetupCompressReaders() {
	var crs = initramfs.CompressReaders

	crs[initramfs.Xz] = XzReader
	crs[initramfs.Zstd] = ZstdReader
}
