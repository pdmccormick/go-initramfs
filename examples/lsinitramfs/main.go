package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"go.pdmccormick.com/initramfs"
)

func setupCompressReaders() {
	var crs = initramfs.CompressReaders

	crs[initramfs.Xz] = func(r io.Reader) (io.Reader, error) { return xz.NewReader(r) }
	crs[initramfs.Zstd] = func(r io.Reader) (io.Reader, error) { return zstd.NewReader(r) }
}

var (
	hideTrailerFlag  = flag.Bool("T", false, "hide trailer entry")
	hideCompressFlag = flag.Bool("C", false, "hide start of compression")
)

func main() {
	flag.Parse()

	var name = flag.Args()[0]

	f, err := os.Open(name)
	if err != nil {
		log.Fatalf("Open: %s", err)
	}

	defer f.Close()

	setupCompressReaders()

	var r = initramfs.NewReader(f)
	if err := list(os.Stdout, r); err != nil {
		log.Fatal(err)
	}
}

func list(out io.Writer, r *initramfs.Reader) error {
Loop:
	for {
		for _, hdr := range r.All() {
			if hdr.Trailer() && *hideTrailerFlag {
				continue
			}

			var suffix string

			if hdr.Mode.Symlink() {
				data, err := io.ReadAll(r)
				if err == nil {
					suffix = fmt.Sprintf(" -> %s", string(data))
				}
			}

			fmt.Fprintf(out, "%s %4d  %4d %4d  %8d  %s  %s%s\n", hdr.Mode, hdr.NumLinks, hdr.Uid, hdr.Gid, hdr.DataSize, hdr.Mtime, hdr.Filename, suffix)

			if hdr.Trailer() {
				fmt.Fprintf(out, "\n")
			}
		}

		if compressed, typ, err := r.ContinueCompressed(nil); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		} else if compressed {
			if !*hideCompressFlag {
				fmt.Printf("# compression %s\n\n", typ.String())
			}
			continue Loop
		} else {
			break Loop
		}
	}
	return nil
}
