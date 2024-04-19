package main

import (
	"encoding/hex"
	"encoding/json"
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

func main() {
	var (
		inFlag      = flag.String("i", "", "read input archive from `file`name (leave blank for stdin)")
		hexDumpFlag = flag.Bool("hexdump", false, "hex dump up to 512 bytes from each file")
	)

	flag.Parse()

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

	var ir = initramfs.NewReader(in)

	var proc = Processor{W: os.Stdout}
	proc.start()

	if err := proc.Scan(ir, *hexDumpFlag); err != nil {
		panic(err)
	}

	proc.stop()
}

type Processor struct {
	W        io.Writer
	notFirst bool
}

type CompressionEntry struct {
	Compression string `json:"Compression"`
}

func (p *Processor) start() {
	fmt.Fprintf(p.W, "[\n")
}

func (p *Processor) stop() {
	fmt.Fprintf(p.W, "\n]\n")
}

func (p *Processor) emitEntry(entry any) error {
	const (
		Prefix = "  "
		Indent = "  "
	)

	data, err := json.MarshalIndent(entry, Prefix, Indent)
	if err != nil {
		return fmt.Errorf("json: %w", err)
	}

	var carry string
	if p.notFirst {
		carry = ",\n"
	}

	fmt.Fprintf(p.W, "%s"+Prefix+"%s", carry, string(data))

	p.notFirst = true
	return nil
}

func (p *Processor) Scan(r *initramfs.Reader, dumpHex bool) error {
Loop:
	for {
		hdr, err := r.Next()
		switch {
		case err == initramfs.ErrCompressedContentAhead:
			if compressed, typ, err := r.ContinueCompressed(nil); err != nil {
				return err
			} else if typ.EOF() {
				return nil
			} else if compressed {
				if err := p.emitEntry(CompressionEntry{Compression: typ.String()}); err != nil {
					return err
				}
				continue Loop
			} else {
				break Loop
			}
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		p.emitEntry(hdr)

		if dumpHex && hdr.DataSize > 0 {
			var data [512]byte
			if n, err := r.Read(data[:]); err != nil {
				return err
			} else if n > 0 {
				fmt.Println("\n" + hex.Dump(data[:n]))
			}
		}
	}

	return nil
}
