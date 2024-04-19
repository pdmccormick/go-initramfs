package initramfs

// Current practise is to align Intel x86 kernel microcode update data to a 16
// byte boundary, although this may only be necessary for older kernel versions.
//
// Use with [Writer.SetDataAlignment].
const MicrocodeDataAlignment = 16

// The Linux kernel can load x86 microcode updates from an initramfs very early
// in the boot process. See [The Linux Microcode Loader] and the
// [go.pdmccormick.com/initramfs/examples/earlyinitramfs] example.
//
// [The Linux Microcode Loader]: https://www.kernel.org/doc/html/latest/arch/x86/microcode.html
const (
	MicrocodeX86Path           = "kernel/x86/microcode/"
	MicrocodePath_AuthenticAMD = "kernel/x86/microcode/AuthenticAMD.bin"
	MicrocodePath_GenuineIntel = "kernel/x86/microcode/GenuineIntel.bin"
)
