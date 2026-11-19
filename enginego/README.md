# kizuna/enginego

Go shared library that drives visual novels from the KizunaEngine GUI.

## Purpose

`enginego/` is a single Go module (`kizuna/enginego`, `go 1.27.2`) that opens a game
directory, runs its scripts, and exposes the runtime state through one C ABI.
The Flutter side (`app` via `lib/core/bridge`) is the sole consumer; it speaks to
the engine through `libenginego.so` and never sees any of the brand-specific file
format work. The engine has no UI, so it builds in isolation from the GUI and can
be hot-swapped on Android. Beyond the standard library it depends on
`golang.org/x/image`, `golang.org/x/text` and `golang.org/x/sys` (pinned in
`go.sum`; `golang.org/x/sys` is indirect, pulled in by the other two).

The module is brand-agnostic. `core` defines the engine contract (`Engine`)
and the brand registry (`Brand`); one directory under `brand/` holds each
engine family and may contain subpackages. Registered families are
`brand/bishop` (five variants: `bishop`, `bishop-legacy`, `bishop-gothic`,
`bishop-tj2`, `bishop-bdt`), `brand/emon`, and `brand/overdose` (ARC + ASB + CPB).
A new engine is added by creating
one brand
package that calls `core.Register` from its `init()` and blank-importing it from
`drivers/drivers.go` — nothing under `cmd/` or in the wire protocol is
engine-specific.

Over-Dose accepts UTF-8 ASB strings as well as the original CP932. A translated
script may be dropped as `<game>/<name>.asb`, `<game>/script/<name>.asb`,
`<game>/scripts/<name>.asb`, `<game>/translated/<name>.asb` or
`<game>/translation/<name>.asb`; loose files take precedence over `script.arc`.
Both the encrypted ASB container and a decompressed instruction stream are
accepted.

Runtime log: `<cacheRoot>/<md5(dataDir)>/engine.log` (same per-game directory as
materialized audio). The log records archive counts, script loads, bg/CG changes,
choice menus, save/load and unknown presentation ops.

## Build outputs

`make host` produces `enginego/libenginego.so` (and the matching
`libenginego.h`) on the build host. `make android` produces three Android variants:
`libenginego-android-arm64.so`, `libenginego-android-arm32.so` and
`libenginego-android-amd64.so`. CI copies them to
`app/android/app/src/main/jniLibs/<abi>/libenginego.so` so they end up bundled in
the split APKs at `lib/<abi>/libenginego.so`. At runtime the Flutter bridge
(`NativeEngineBindings.open`) loads `libenginego.so` from the APK.

On Windows/Linux CI builds a c-shared `enginego.dll`/`libenginego.so`; on Apple
platforms `cmd/engine` cannot be used directly (iOS has no c-shared and two Go
archives would clash), so the root `bridge/` module links `enginego/capi` and
`kernel/shell` into one c-archive. All dispatch logic lives in `capi`; the two
`cmd` entry points are thin cgo shims.

`make test`, `make vet`, `make fmt`, `make clean` mirror the kernel workflow.

## C ABI

Five exported symbols, defined by thin cgo shims in package `main`
(`enginego/cmd/engine` on Android/desktop, `bridge/cmd/bridge` on Apple). The
implementation lives in `enginego/capi` and is shared by both shims:

* `KzEngineCreate() unsafe.Pointer` — allocates one opaque handle slot in **C
  memory** (`C.malloc(1)`), uses its address as the key of a mutex-guarded
  `map[uintptr]*handle`, and returns that pointer. The handle owns no engine
  yet; the engine is created lazily by `OpOpen` once the brand is resolved.
  The returned pointer is the only way to refer to that engine instance.
* `KzEngineDestroy(h unsafe.Pointer)` — removes the handle from the registry
  and `C.free`s the slot; a repeated call with the same pointer is a no-op.
  The engine itself is garbage-collected once the map drops its reference.
* `KzEngineInvoke(h, req, reqLen, outLen) unsafe.Pointer` — parses one request,
  dispatches one op against the engine, copies the response bytes into a
  `C.malloc`'d buffer, writes the buffer length through `*outLen`, and returns
  the buffer. The caller frees it with `KzEngineFree`. A `defer recover()` in
  every exported function and the per-request dispatch ensures a panic becomes
  an error response rather than crossing the boundary.
* `KzEngineFree(p)` — calls `C.free(p)` for buffers and strings returned by the
  other four functions.
* `KzEngineVersion() *C.char` — returns `C.CString("kizuna-engine/1.0.0")`. The
  caller frees the string with `KzEngineFree`.

No Go pointer ever crosses the boundary: the handle slot is C memory, the
Go `*handle` (which does contain Go pointers) is reachable only through the Go
map keyed by the slot's address, request slices are read from the caller's
buffer, and every response buffer is `C.malloc`'d. Returning a Go pointer would
abort the whole process with a native fatal error, so this is load-bearing. The
rule also shapes how the ops are implemented: field 42 carries an error message
string, and `OpSlotPreview` stays available because it copies the thumbnail
pixels into a freshly allocated response buffer instead of handing out Go
memory. Slot previews are alive and well — `core.SlotPreviewer`, op 32
(`OpSlotPreview`) and the dispatch in `cmd/engine/main.go` implement them in
full. `core.Slot` carries five fields: `Index`, `Title`, `SavedAt`, `Summary`
and `Exists`.

This is the only file that imports `C`. The rest of the module is pure Go.

## Binary wire protocol

Little-endian everywhere, byte-oriented, no NULs.

Request (16-byte header + flat payload):

```
u8[4]  magic     'K''Z''E''Q'
u16    op
u16    flags     (0, reserved)
u32    seq
u32    payloadLen
u8[payloadLen] payload   (a flat sequence of fields)
```

Response (16-byte header + flat payload):

```
u8[4]  magic     'K''Z''E''S'
u16    status    0 ok, 1 error, 2 notFound, 3 badRequest
u16    reserved  (0)
u32    seq       (echo)
u32    payloadLen
u8[payloadLen] payload
```

Field (8-byte head + value):

```
u16    id
u8     type      1 u32, 2 i32, 3 u64, 4 str, 5 bytes, 6 list, 7 bool, 8 f64
u8     reserved  (0)
u32    len
u8[len] value
```

A `list` field's value is itself a flat sequence of fields (recursive). Readers
must skip unknown ids and tolerate missing fields (return the zero value). Op
ids, request field ids, and response field ids are fixed by `.task/PROTOCOL.md`
§5 — see `wire/op.go` and the dispatch in `cmd/engine/main.go`.

Ops 21 (`OpBrands`) and 22 (`OpDetect`) are brand-generic: `OpBrands` takes no
request fields and answers field 18 with a list of items (field 19 = brand id,
field 20 = brand name); `OpDetect` takes field 1 = directory path and answers
fields 19/20 (empty when no brand matches). `OpOpen` additionally accepts
field 44 = brand id (string, optional): when non-empty the brand is looked up
directly instead of detected, and an unknown id is an error response. It also
reads field 7 = `extraDirs` (a list of directory strings) and field 10 =
`cacheDir` (a string, the materialised-audio cache directory); both are passed
to `core.OpenerExtras.OpenExtras` when the brand implements that interface.

Wire field ids 42–45: 42 is an error message string (every error response and
`OpLastError` carry it), 43 is unused by both the Go engine and the Dart client,
44 is the `OpOpen` brand id, and 45 is `OpState`'s `waitMs` (u32).

`wire.Writer` is reusable (`Reset()` keeps the backing array) and walks the
list/tree exactly once per response. `wire.Reader` is a zero-copy view over
the request bytes: it builds a `map[uint16]field` once and serves `U32`,
`I32`, `U64`, `F64`, `Bool`, `Str` from that map.

## Brand registry

`core/registry.go` holds the brand table: `Register` (duplicate ids are
ignored), `Brands` (copy of the list), `Find` (by id), `Detect` (first brand
whose `Detect(dir)` returns true). A brand implements `core.Brand` and is
registered from its package `init()`. `drivers/drivers.go` blank-imports every
brand package, and `cmd/engine/main.go` blank-imports `drivers`, mirroring
`kernel/cmd/kernel/main.go`.

## Ops before Open

The handle owns no engine until `OpOpen` resolves a brand. Until then the 13
capability ops 24–36 (`OpSubmitName`, `OpAudioDone`, `OpMapButtons`, `OpMapMask`,
`OpSelectItems`, `OpSelectFrame`, `OpComposeImages`, `OpSetPreview`,
`OpSlotPreview`, `OpCgCatalog`, `OpSetOption`, `OpKey`, `OpWheel`) answer an
empty `StatusOK` response — an engine that does not support the capability never
errors. Every other op apart from `OpBrands`/`OpDetect`/`OpOpen` answers an error
response (`engine: not open`). When `OpOpen` fails, the half-open engine is
still stored on the handle so `OpLastError` reports the message.

## Package map

```
enginego/
  go.mod                   module kizuna/enginego, go 1.27.2
  Makefile                 host / android / test / vet / fmt / clean
  README.md                this file
  cmd/engine/main.go       cgo C ABI (only file using cgo), brand-agnostic dispatch
  drivers/drivers.go       blank-imports every brand package
  library/                 game-directory scan for op 23 (Scan / Describe / EngineKind)
  core/                    engine contract + registry + cross-cutting helpers
    engine.go              Engine interface and the shared Step/Info/Scene/Slot/
                           AudioEvent/TextState/Choice/State/TextUnit types
    capabilities.go        optional capability interfaces probed by dispatch
                           (OpenerExtras / NamePrompter / AudioNotifier /
                           MapExposer / SelectItemExposer / GalleryComposer /
                           SlotPreviewer / OptionSetter / KeyReceiver /
                           WheelReceiver / Closer)
    registry.go            Brand interface, Register / Brands / Find / Detect
    log.go                 Logf / SetLogger (discard by default)
    paths.go               case-insensitive path helpers (HasFoldSuffix /
                           TrimFoldSuffix / LowerExt / HasFoldPrefix / Stem /
                           TrimExt / Join / EnsureParentDir)
    fileio.go              ReadFile / WriteAll / AtomicWrite / PathExists /
                           DirExists / FileExists / ReadFileString / ListFilesTop
    num.go                 Clamp / ClampInt32 / Abs / AbsInt32
    compress.go            InflateExact (raw DEFLATE, zlib fallback)
    textcodec/             CP932 / GBK / UTF-16LE / UTF-8 decode + encode +
                           detection, code tables in textcodec/tables
    binutil/               little-endian readers / writers, .NET 7-bit encoded
                           int, MSB bit reader
    strutil/               text normalisation, tolerant integer parsing, split /
                           trim, path stem and extension helpers
    dotnet/                ported .NET System.Random
  brand/bishop/            Bishop brand: core.Brand implementation (register.go)
    gfx/ audio/ config/ data/ rendering/ resource/ runtime/ save/ state/ vm/
                           BSXScript / BSScript / SCF / DSK / image / audio
                           decoders shared by the four registered variants
    session/{legacy,gothic,tj2,bdt}/
                           the four Bishop engine sessions, each registering its
                           own core.Brand
  brand/emon/              EmonEngine brand (core.Register lives in session/)
    eme/ emecfg/ gfx/ glyph/ script/ save/ session/ stage/ text/ vm/
                           EME archive, ECF config, sprite / text pipeline
  wire/                    binary framing
    op.go                  op + status constants (36 ops, 4 statuses)
    writer.go              wire.Writer with Reset() / Str / U32 / I32 / U64 /
                           F64 / Bool / Raw / List
    reader.go              wire.Reader (zero-copy view, typed accessors)
    message.go             ParseRequest / BuildResponse / Error
```

Every brand package is a stateless format helper except `session/`, which owns
the engine state; `cmd/engine` only knows `core.Engine` and `core.Brand`.
Image decoders refuse dimensions above 16384 px and allocations above 512 MiB.
