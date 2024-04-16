// Read and write Linux kernel initramfs-style cpio "newc" formatted archives.
//
// This implementation follows the [documented kernel buffer format]. See also
// [early userspace support] for more information about how the kernel uses
// initramfs during the boot process.
//
// See [go.pdmccormick.com/initramfs/examples] for demonstrations of how to use
// this package.
//
// [documented kernel buffer format]: https://www.kernel.org/doc/html/latest/driver-api/early-userspace/buffer-format.html
// [early userspace support]: https://www.kernel.org/doc/html/latest/driver-api/early-userspace/early_userspace_support.html
package initramfs
