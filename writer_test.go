package initramfs

import "testing"

func TestWriter_ParentDirs(t *testing.T) {
	t.Run("trailer", func(t *testing.T) {
		w, r := testWriterReader(t)

		w.WriteTrailer()

		var hdrs headerList
		hdrs.readAll(r)
		hdrs.expectNames(t,
			TrailerFilename,
		)
	})

	t.Run("dirs", func(t *testing.T) {
		w, r := testWriterReader(t)

		testMkdirAll(t, w, "/lib/modules/kernel/fs", 0o700)
		testMkdirHeader(t, w, "/lib/modules/kernel/drivers", nil)
		testMkdirAll(t, w, "/lib/modules/kernel/drivers/net", 0o700)

		var hdr = Header{
			Mode:     Mode_File | 0o440,
			Filename: "/lib/modules/kernel/drivers/net/e1000.ko",
		}
		testWriteHeader(t, w, &hdr)

		var hdrs headerList
		hdrs.readAll(r)
		hdrs.expectNames(t,
			".",
			"lib",
			"lib/modules",
			"lib/modules/kernel",
			"lib/modules/kernel/fs",
			"lib/modules/kernel/drivers",
			"lib/modules/kernel/drivers/net",
			"lib/modules/kernel/drivers/net/e1000.ko",
		)

	})
}
