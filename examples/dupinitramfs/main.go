package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"go.pdmccormick.com/initramfs"
)

func setupCompressReaders() {
	var crs = initramfs.CompressReaders

	crs[initramfs.Xz] = func(r io.Reader) (io.Reader, error) { return xz.NewReader(r) }
	crs[initramfs.Zstd] = func(r io.Reader) (io.Reader, error) { return zstd.NewReader(r) }
}

func main() {
	var (
		inFlag     = flag.String("i", "", "read input archive from `file`name (leave blank for stdin)")
		outFmtFlag = flag.String("o", "out%d.initramfs", "output `filename` (%d will be expanded for concatenated archives)")
	)

	flag.Parse()

	if *outFmtFlag == "" {
		fmt.Println("missing `-out`")
		flag.Usage()
	}

	var in = os.Stdin

	if name := *inFlag; name != "" {
		f, err := os.Open(name)
		if err != nil {
			log.Fatalf("Open: %s", err)
		}

		defer f.Close()

		in = f
	}

	setupCompressReaders()

	var dup = Dup{
		OutNameFmt: *outFmtFlag,
	}

	if err := dup.Process(in); err != nil {
		log.Fatalf("%s", err)
	}
}

type Dup struct {
	OutNameFmt string

	outName string
	outFile *os.File
}

func (dup *Dup) openOutput(index int) (*os.File, error) {
	name := fmt.Sprintf(dup.OutNameFmt, index)

	// Did not contain printf-style directive
	if strings.Contains(name, "%!(EXTRA int=") {
		name = dup.OutNameFmt
	}

	// A file with this name is already open, nothing more to do
	if name == dup.outName && dup.outFile != nil {
		return dup.outFile, nil
	}

	if dup.outFile != nil {
		dup.outFile.Close()
	}

	f, err := os.Create(name)
	if err != nil {
		return nil, fmt.Errorf("Create %s: %w", name, err)
	}

	dup.outName = name
	dup.outFile = f

	log.Printf("Writing %s", dup.outName)

	return f, nil
}

func (dup *Dup) Close() {
	if f := dup.outFile; f != nil {
		f.Close()
		dup.outFile = nil
	}
}

func (dup *Dup) Process(r io.Reader) error {
	var ir = initramfs.NewReader(r)

	defer dup.Close()

	for i := 0; ; i++ {
		w, err := dup.openOutput(i)
		if err != nil {
			return err
		}

		var iw = initramfs.NewWriter(w)

		defer iw.Close()

		if err := copyInitramfs(ir, iw); err != nil {
			return err
		}

		if err := iw.WriteTrailer(); err != nil {
			return fmt.Errorf("WriteTrailer: %w", err)
		}

		isCompressed, compressType, err := ir.ContinueCompressed(nil)
		if err != nil {
			if err == io.EOF {
				break
			}

			return fmt.Errorf("ContinueCompressed: %w", err)
		}

		if isCompressed {
			log.Printf("Found %s compressed stream", compressType)
		}
	}

	return nil
}

func copyInitramfs(r *initramfs.Reader, w *initramfs.Writer) error {
	for _, hdr := range r.All() {
		if hdr.Trailer() {
			break
		}

		if err := w.WriteHeader(&hdr); err != nil {
			return fmt.Errorf("WriteHeader: %w", err)
		}

		if hdr.DataSize > 0 {
			if _, err := io.Copy(w, r); err != nil {
				return fmt.Errorf("Copy %s: %w", hdr.Filename, err)
			}
		}

		fmt.Printf(">\t%s\n", &hdr)
	}

	return nil
}
