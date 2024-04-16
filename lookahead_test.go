package initramfs

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

func TestPeekLookahead(t *testing.T) {
	var testcases = []struct {
		name string
		la   Lookahead
	}{
		{"EOF", EOF},
		{"testdata/data.cpio", CpioFile},
		{"testdata/data.cpio.prepadded", Padding},
		{"testdata/data.cpio.gz", Gzip},
		{"testdata/data.cpio.bz2", Bzip2},
		{"testdata/data.cpio.lzma", Lzma},
		{"testdata/data.cpio.xz", Xz},
		{"testdata/data.cpio.lzo", Lzo},
		{"testdata/data.cpio.lz4", Lz4},
		{"testdata/data.cpio.zstd", Zstd},
	}

	for i, tc := range testcases {
		var r io.Reader
		if tc.name == "EOF" {
			// Will EOF immediately
			r = &io.LimitedReader{R: nil, N: 0}
		} else {
			r = bytes.NewReader(readTestdata(t, tc.name))
		}

		var br = bufio.NewReader(r)
		la, err := PeekLookahead(br)
		if err != nil {
			t.Errorf("#%d: error: %s", i, err)
			continue
		}

		if expect, got := tc.la, la; expect != got {
			t.Errorf("#%d: expected %s, got %s", i, expect, got)
			continue
		}
	}
}
