GO ?= go
IOS_SDK ?= $(shell xcrun --sdk iphoneos --show-sdk-path)
IOS_MIN ?= 15.0

# Apple 静态库：engine + kernel 合成一份 Go 运行时（原因见 cmd/bridge/main.go）。
#
# 注意不要给 c-archive 加 -ldflags=-s：-s 会丢掉符号表，Xcode 链接时
# 需要从 .a 里解析 KzEngine* / Kz* 符号，丢了就直接 link 失败。
.PHONY: darwin ios test vet clean

darwin:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC="clang -arch arm64" \
		$(GO) build -buildmode=c-archive -trimpath -ldflags='-w' -o libkizuna-darwin-arm64.a ./cmd/bridge
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CC="clang -arch x86_64" \
		$(GO) build -buildmode=c-archive -trimpath -ldflags='-w' -o libkizuna-darwin-amd64.a ./cmd/bridge
	lipo -create -output libkizuna.a libkizuna-darwin-arm64.a libkizuna-darwin-amd64.a
	lipo -info libkizuna.a

ios:
	GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
		CC="$(shell xcrun --sdk iphoneos -f clang)" \
		CGO_CFLAGS="-isysroot $(IOS_SDK) -arch arm64 -miphoneos-version-min=$(IOS_MIN)" \
		CGO_LDFLAGS="-isysroot $(IOS_SDK) -arch arm64 -miphoneos-version-min=$(IOS_MIN)" \
		$(GO) build -buildmode=c-archive -trimpath -ldflags='-w' -o libkizuna-ios-arm64.a ./cmd/bridge
	lipo -info libkizuna-ios-arm64.a

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

clean:
	rm -f libkizuna*.a libkizuna*.h
