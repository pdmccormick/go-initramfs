#!/bin/bash

# On a Debian/Ubuntu system, you will need to install the following packages:
#
#       apt-get install -y gzip bzip2 xz-utils lzop lz4 zstd

set -e

touch_date() {
    NAME="$1"
    touch --date='Mon Jan 2 15:04:05 MST 2006' "$NAME"
}

echo -ne "Hello World!\n" > helloworld.txt
touch_date helloworld.txt

echo helloworld.txt | cpio -H newc -ov > data.cpio
touch_date data.cpio

( dd if=/dev/zero bs=512 count=1 ; cat data.cpio ) > data.cpio.prepadded

function compress() {
    SRC="$1"
    SUFFIX="$2"
    INVOKE="$3"

    $INVOKE data.cpio > "data.cpio${SUFFIX}"
}

compress "data.cpio"    ".gz"       "gzip -c -n"
compress "data.cpio"    ".bz2"      "bzip2 -c"
compress "data.cpio"    ".lzma"     "lzma -c"
compress "data.cpio"    ".xz"       "xz --check=crc32 -9 --lzma2=dict=1MiB -c"
compress "data.cpio"    ".lzo"      "lzop -9 -c"
compress "data.cpio"    ".lz4"      "lz4 -2 -l -c"
compress "data.cpio"    ".zstd"     "zstd -q -1 -T0 -c"
