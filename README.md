initramfs
=========

Read and write Linux kernel initramfs-style cpio "newc" formatted archives.

```go
import "go.pdmccormick.com/initramfs"
```

This implementation follows the [documented kernel buffer format](https://www.kernel.org/doc/html/latest/driver-api/early-userspace/buffer-format.html).
See also [early userspace support](https://www.kernel.org/doc/html/latest/driver-api/early-userspace/early_userspace_support.html) for more information about how the kernel uses initramfs during the boot process.

See [examples](./examples) for demonstrations of how to use this package.
