package initramfs

import (
	"fmt"
	"testing"
	"time"
)

func timeParse(t *testing.T, v string) time.Time {
	tm, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("Time parse `%s`: %s", v, err)
	}
	return tm
}

func TestHeader_ReadFrom(t *testing.T) {
	var testcases = []struct {
		name         string
		expectHeader Header
	}{
		{
			name: "testdata/header-microcode.cpio",
			expectHeader: Header{
				Magic:        Magic_070701,
				Inode:        4,
				Mode:         0o100_600,
				Uid:          0,
				Gid:          0,
				NumLinks:     1,
				Mtime:        timeParse(t, "2019-12-17T19:00:00-05:00"),
				DataSize:     76166,
				Major:        0,
				Minor:        0,
				RMajor:       0,
				RMinor:       0,
				FilenameSize: 38,
				Checksum:     0,
				Filename:     "kernel/x86/microcode/AuthenticAMD.bin",
			},
		},
		{
			name: "testdata/header-tty1.cpio",
			expectHeader: Header{
				Magic:        Magic_070701,
				Inode:        21,
				Mode:         0o_020_620,
				Uid:          122,
				Gid:          5,
				NumLinks:     1,
				Mtime:        timeParse(t, "2024-03-14T04:22:28-04:00"),
				DataSize:     0,
				Major:        0,
				Minor:        5,
				RMajor:       4,
				RMinor:       1,
				FilenameSize: 10,
				Checksum:     0,
				Filename:     "/dev/tty1",
			},
		},
	}

	for i, tc := range testcases {
		t.Run(fmt.Sprintf("#%d %s", i, tc.name), func(t *testing.T) {
			var (
				got Header
				r   = testdataReader(t, tc.name)
			)

			if _, err := got.ReadFrom(r); err != nil {
				t.Fatalf("Header ReadFrom: %s", err)
			}

			if got != tc.expectHeader {
				t.Fatalf("Mismatch, expected %+v, got %+v", tc.expectHeader, got)
			}
		})
	}
}
