#!/bin/bash
set -x

opt="-Wall -g -O2 -fPIC -flto -fwhole-program -fno-use-linker-plugin"
TARGET="$PWD/internal/pkg/chelper/libc_helper.so"

rm -rf ./libc_helper.so ./libc_helper.a
$CC $opt -shared -o $PWD/chelper/libc_helper.so $PWD/chelper/*.c
# Also create static library
$CC $opt -c $PWD/chelper/*.c
ar rcs $PWD/chelper/libc_helper.a *.o
rm *.o
mv $PWD/chelper/libc_helper.so $PWD/internal/pkg/chelper/
mv $PWD/chelper/libc_helper.a $PWD/internal/pkg/chelper/
