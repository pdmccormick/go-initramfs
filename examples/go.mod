module go.pdmccormick.com/initramfs/examples

replace go.pdmccormick.com/initramfs => ../

go 1.23.0

require (
	github.com/klauspost/compress v1.17.8
	go.pdmccormick.com/initramfs v0.0.0-00010101000000-000000000000
)

require github.com/ulikunitz/xz v0.5.12
