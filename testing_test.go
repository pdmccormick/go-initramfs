package initramfs

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"slices"
	"testing"
)

//go:embed testdata
var testdata embed.FS

func readTestdata(t *testing.T, name string) []byte {
	data, err := fs.ReadFile(testdata, name)
	if err != nil {
		t.Fatalf("readTestdata %s: %s", name, err)
	}
	return data
}

func testdataReader(t *testing.T, name string) io.Reader {
	r, err := testdata.Open(name)
	if err != nil {
		t.Fatalf("testdataReader %s: %s", name, err)
	}
	return r
}

func testWriterReader(t *testing.T) (*Writer, *Reader) {
	var (
		b bytes.Buffer
		w = NewWriter(&b)
		r = NewReader(&b)
	)

	return w, r
}

func testWriteHeader(t *testing.T, w *Writer, hdr *Header) {
	if err := w.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader %+v: %s", hdr, err)
	}
}

func testMkdirHeader(t *testing.T, w *Writer, filename string, template *Header) {
	var hdr = Header{
		Mode: Mode_Dir | 0o755,
	}

	if template != nil {
		hdr = *template
	}

	hdr.Filename = filename

	if err := w.WriteHeader(&hdr); err != nil {
		t.Fatalf("WriteHeader %+v: %s", hdr, err)
	}
}

func testMkdirAll(t *testing.T, w *Writer, path string, perm Mode) {
	if err := w.MkdirAll(path, perm); err != nil {
		t.Fatalf("MkdirAll %s %s: %s", path, perm, err)
	}
}

type headerList []Header

func (hdrs *headerList) readAll(r *Reader) {
	for _, hdr := range r.All() {
		*hdrs = append(*hdrs, hdr)
	}
}

func (hdrs headerList) expectNames(t *testing.T, names ...string) {
	var got = make([]string, len(hdrs))

	for i, hdr := range hdrs {
		got[i] = hdr.Filename
	}

	if !slices.Equal(names, got) {
		t.Errorf("expected names %v, got %v", names, got)
	}
}
