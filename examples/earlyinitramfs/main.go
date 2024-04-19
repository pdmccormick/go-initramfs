// Generates an initramfs archive for early microcode loading according to reference in [The Linux Microcode Loader].
//
// [The Linux Microcode Loader]: https://www.kernel.org/doc/html/latest/arch/x86/microcode.html
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"go.pdmccormick.com/initramfs"
)

func main() {
	var (
		outFlag       = flag.String("o", "early.initramfs", "write output archive to `file`name (use '-' for stdout)")
		amdGlobFlag   = flag.String("amdglob", "/lib/firmware/amd-ucode/microcode_amd*.bin", "glob `pattern` for AMD microcode files")
		intelGlobFlag = flag.String("intelglob", "/lib/firmware/intel-ucode/*", "glob `pattern` for Intel microcode files")
	)

	flag.Parse()

	var w io.Writer

	if name := *outFlag; name == "-" {
		w = os.Stdout
	} else if name != "" {
		out, err := os.Create(name)
		if err != nil {
			log.Fatalf("Create: %s", err)
		}

		defer out.Close()
		w = out

		log.Printf("Writing %s", name)
	}

	var (
		pr, pw  = io.Pipe()
		writers = [...]io.Writer{w, pw}
	)

	go func() {
		var ir = initramfs.NewReader(pr)
		for _, hdr := range ir.All() {
			log.Printf("\t%s", &hdr)
		}
	}()

	var (
		mw = io.MultiWriter(writers[:]...)
	)

	var iw = initramfs.NewWriter(mw)

	defer iw.Close()

	if err := writeEarly(iw, *amdGlobFlag, *intelGlobFlag); err != nil {
		log.Fatalf("%s", err)
	}
}

func writeEarly(iw *initramfs.Writer, amdGlob, intelGlob string) error {
	var files = []struct {
		SrcPattern string
		Dst        string
		Alignment  int
	}{
		{amdGlob, initramfs.MicrocodePath_AuthenticAMD, 0},
		{intelGlob, initramfs.MicrocodePath_GenuineIntel, initramfs.MicrocodeDataAlignment},
	}

	for _, file := range files {
		if file.SrcPattern == "" {
			continue
		}

		var data = new(bytes.Buffer)
		matches, err := concatenateAll(file.SrcPattern, data)
		if err != nil {
			return err
		}

		if len(matches) == 0 {
			log.Printf("No files matching %s found, skipping %s", file.SrcPattern, file.Dst)
			continue
		}

		log.Printf("Concatenating %d files (%d bytes total) from %s", len(matches), data.Len(), file.SrcPattern)

		var (
			mode = initramfs.Mode_File.WithPerms(0o664)
			hdr  = initramfs.Header{
				Filename: file.Dst,
				Mode:     mode,
				DataSize: uint32(data.Len()),
			}
		)

		if alignTo := file.Alignment; alignTo > 0 {
			if err := iw.SetDataAlignment(alignTo); err != nil {
				return fmt.Errorf("SetDataAlignment %d: %w", alignTo, err)
			}
		}

		if err := iw.WriteHeader(&hdr); err != nil {
			return fmt.Errorf("WriteHeader: %w", err)
		}

		if _, err := iw.ReadFrom(data); err != nil {
			return fmt.Errorf("ReadFrom: %w", err)
		}
	}

	if err := iw.WriteTrailer(); err != nil {
		return err
	}

	return nil
}

func concatenateAll(pattern string, out *bytes.Buffer) (matches []string, err error) {
	matches, err = filepath.Glob(pattern)
	if err != nil {
		err = fmt.Errorf("Glob %s: %w", pattern, err)
		return
	}

	for _, name := range matches {
		var f *os.File
		f, err = os.Open(name)
		if err != nil {
			return nil, fmt.Errorf("Open %s: %w", name, err)
		}

		_, err = io.Copy(out, f)

		f.Close()

		if err != nil {
			return
		}
	}

	return
}
