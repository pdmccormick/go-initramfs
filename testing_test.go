package initramfs

import (
	"embed"
	"io"
	"io/fs"
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
