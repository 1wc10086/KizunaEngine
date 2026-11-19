GO ?= go
NDK := $(or $(ANDROID_NDK_LATEST_HOME),$(ANDROID_NDK_ROOT))
ifeq ($(strip $(NDK)),)
NDK := $(lastword $(sort $(wildcard $(ANDROID_HOME)/ndk/*)))
endif

# 三种 Android ABI 的 NDK clang。API 23 是 Flutter 当前的最低目标；
# 三元组与 jniLibs 目录名一一对应（arm32 用 armeabi-v7a）。
NDK_BIN := $(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin
CC_ARM64 := $(NDK_BIN)/aarch64-linux-android23-clang
CC_ARM32 := $(NDK_BIN)/armv7a-linux-androideabi23-clang
CC_AMD64 := $(NDK_BIN)/x86_64-linux-android23-clang

# 模块从 engine 改名 enginego 后，产物也统一带 enginego：Flutter 侧按
# `_tryOpen('enginego', ...)` 的基名加载 libenginego.so / enginego.dll。
WINDOWS_CC ?= x86_64-w64-mingw32-gcc

.PHONY: all host linux-amd64 darwin-universal windows-amd64 ios android android-arm64 android-arm32 android-amd64 test vet fmt clean

all: host

host:
	$(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego.so ./cmd/engine

linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-linux-amd64.so ./cmd/engine

# macOS 的 c-shared 是 .dylib；两个架构各编一份再用 lipo 合成通用库。
# 最终产物叫 libenginego.dylib，与 Flutter 侧按基名查找的布局一致。
darwin-universal:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="clang -arch arm64" \
		$(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-darwin-arm64.dylib ./cmd/engine
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="clang -arch x86_64" \
		$(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-darwin-amd64.dylib ./cmd/engine
	lipo -create -output libenginego.dylib libenginego-darwin-arm64.dylib libenginego-darwin-amd64.dylib
	lipo -info libenginego.dylib

windows-amd64:
	command -v $(WINDOWS_CC) >/dev/null 2>&1 || (echo "missing mingw gcc: $(WINDOWS_CC)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="$(WINDOWS_CC)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o enginego.dll ./cmd/engine

# iOS 没有 c-shared，而且引擎必须和 kernel 合成一个 Go 运行时（两个
# c-archive 会撞运行时符号），所以组合库由 bridge/Makefile 产出
# libkizuna-ios-arm64.a；这里只转发，避免两份重复的交叉编译参数。
ios:
	$(MAKE) -C ../bridge ios

android: android-arm64 android-arm32 android-amd64

android-arm64:
	test -x "$(CC_ARM64)" || (echo "missing NDK clang: $(CC_ARM64)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$(CC_ARM64)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-android-arm64.so ./cmd/engine

android-arm32:
	test -x "$(CC_ARM32)" || (echo "missing NDK clang: $(CC_ARM32)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=arm GOARM=7 CC="$(CC_ARM32)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-android-arm32.so ./cmd/engine

android-amd64:
	test -x "$(CC_AMD64)" || (echo "missing NDK clang: $(CC_AMD64)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$(CC_AMD64)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libenginego-android-amd64.so ./cmd/engine

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l .

clean:
	rm -f libenginego.so libenginego.h libenginego-android-*.so libenginego-android-*.h
	rm -f libenginego-linux-amd64.so libenginego-linux-amd64.h
	rm -f libenginego.dylib libenginego-darwin-*
	rm -f enginego.dll enginego.h
