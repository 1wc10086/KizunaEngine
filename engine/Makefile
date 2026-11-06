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

.PHONY: all host android android-arm64 android-arm32 android-amd64 test vet fmt clean

all: host

host:
	$(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libengine.so ./cmd/engine

android: android-arm64 android-arm32 android-amd64

android-arm64:
	test -x "$(CC_ARM64)" || (echo "missing NDK clang: $(CC_ARM64)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$(CC_ARM64)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libengine-android-arm64.so ./cmd/engine

android-arm32:
	test -x "$(CC_ARM32)" || (echo "missing NDK clang: $(CC_ARM32)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=arm GOARM=7 CC="$(CC_ARM32)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libengine-android-arm32.so ./cmd/engine

android-amd64:
	test -x "$(CC_AMD64)" || (echo "missing NDK clang: $(CC_AMD64)" >&2; exit 1)
	CGO_ENABLED=1 GOOS=android GOARCH=amd64 CC="$(CC_AMD64)" $(GO) build -buildmode=c-shared -trimpath -ldflags='-s -w' -o libengine-android-amd64.so ./cmd/engine

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l .

clean:
	rm -f libengine.so libengine.h libengine-android-*.so libengine-android-*.h
