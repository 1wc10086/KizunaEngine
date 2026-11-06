#!/usr/bin/env bash
set -euo pipefail
NDK_PINNED="27.3.13750724"
CC_REL="toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android23-clang"
NDK=""
if [ -n "${ANDROID_HOME:-}" ] && [ -x "$ANDROID_HOME/ndk/$NDK_PINNED/$CC_REL" ]; then
  NDK="$ANDROID_HOME/ndk/$NDK_PINNED"
fi
if [ -z "$NDK" ]; then
  NDK="${ANDROID_NDK_LATEST_HOME:-}"
fi
if [ -z "$NDK" ]; then
  NDK="${ANDROID_NDK_ROOT:-}"
fi
if [ -z "$NDK" ]; then
  NDK="${ANDROID_NDK_HOME:-}"
fi
if [ -z "$NDK" ] && [ -n "${ANDROID_HOME:-}" ]; then
  NDK="$(ls -d "$ANDROID_HOME"/ndk/* 2>/dev/null | sort -V | tail -n 1)"
fi
if [ -z "$NDK" ]; then
  echo "Android NDK not found; set ANDROID_NDK_LATEST_HOME, ANDROID_NDK_ROOT, or ANDROID_HOME" >&2
  exit 1
fi
CC="$NDK/$CC_REL"
if [ ! -x "$CC" ]; then
  echo "NDK clang not found or not executable: $CC" >&2
  exit 1
fi
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LDFLAGS='-s -w -extldflags=-Wl,-z,max-page-size=16384'
(cd "$ROOT/kernel" && CGO_ENABLED=1 GOOS=android GOARCH=arm64 GOFLAGS=-buildvcs=false CC="$CC" go build -buildmode=c-shared -trimpath -ldflags="$LDFLAGS" -o libkernel.so ./cmd/kernel)
ls -la "$ROOT/kernel/libkernel.so"
JNILIBS="$ROOT/app/android/app/src/main/jniLibs/arm64-v8a"
mkdir -p "$JNILIBS"
cp "$ROOT/kernel/libkernel.so" "$JNILIBS/libkernel.so"
ls -la "$JNILIBS/libkernel.so"
