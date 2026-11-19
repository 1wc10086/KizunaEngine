# kernel（Go 内核）

`kernel/` 是 KizunaEngine 的解码 / 编码 / 翻译逻辑承载层：Go 模块（`kizuna/kernel`），编译为
C 共享库 `libkernel.so`，Flutter 侧（`app/lib/core/bridge/kernel_bridge.dart`）通过 C ABI 以 JSON-RPC 方式调用。
Dart 只负责界面与调用，所有文件格式解析、文本编解码、机器翻译流水线均在 Go 内实现。
`libkernel.so` 替换后功能即生效，APK 本体无需重新编译打包。

- 模块：`kizuna/kernel`，`go 1.27.2`，零第三方依赖，只使用 Go 标准库（`go.mod` 无 `require`）。
- 平台：Android/Linux/Windows 编译为 `libkernel.so` / `kernel.dll`；iOS/macOS 由仓库根部的
  `bridge/` 模块与引擎一起编成单个 `libkizuna.a`（两个独立 Go c-archive 会重复定义运行时符号）。
- 线程模型：除翻译任务中心（互斥锁保护）外，`shell.Invoke` 为无状态函数，可并发调用；
  Dart 侧由 `KernelBridge` 直接同步调用（见 `app/lib/core/bridge/kernel_bridge.dart`）。
- 权威接口说明见 `.task/DESIGN.md`（冻结规范）；本文描述代码现状，字段名、行为以代码为准。

## 目录结构

| 路径 | 职责 | 关键导出 |
| --- | --- | --- |
| `core/` | 公共层：feature 注册、文件 / 目录探测、文档模型、编解码通用入口、文件 IO、日志、路径辅助 | `Feature`、`Handler`、`HandlerRef`、`Target`（`TargetFile`/`TargetDirectory`）、`Options`、`Result`；`Register`、`Features`、`Find`、`Handlers`；`Detect`、`DetectFormat`、`Candidate`、`DetectResult`；`Codec`、`Document`、`Unit`、`Section`、`Meta`；`RunCodecDecode`、`RunCodecEncode`、`ProbeFeature`、`ErrNotMine`、`DefaultUnitText`、`DefaultSetUnitText`；`ReadFile`、`AtomicWrite`、`WriteJSON`、`ReadJSON`；`Logger`、`SetLogger`、`Logf`；`HasFoldSuffix`、`TrimFoldSuffix`、`LowerExt`、`EnsureParentDir` |
| `core/textcodec/` | 文本编码独立子包：CP932 / GBK / UTF-16LE / UTF-8 编解码与自动识别，码表内嵌于 `tables/` | `Decode`、`Encode`、`UnitSize`；`DecodeCP932`、`EncodeCP932`、`DecodeGBK`、`EncodeGBK`、`DecodeUTF16LE`、`EncodeUTF16LE`、`DecodeUTF8`；`DetectTextEncoding`（返回 `utf16le` / `utf8` / `cp932` / `gbk`） |
| `shell/` | C ABI 之上的 JSON-RPC 层：请求分发、翻译任务与事件中心 | `Version`（`kizuna-kernel/1.0.0`）、`Invoke`；`TranslationRunner`（`translator.Runner` 的可替换变量，用于测试注入） |
| `cmd/kernel/` | `package main`，Android/桌面的 c-shared 导出点（薄壳） | `KzInvoke`、`KzVersion`、`KzFree` |
| `../bridge/cmd/bridge/` | Apple 平台的组合入口：与引擎共用一份 Go 运行时，导出同一组 `Kz*` | `KzInvoke`、`KzVersion`、`KzFree` |
| `drivers/` | 空导入全部格式包，以 `init()` 触发注册 | 无导出；导入 `bdt`、`bsscript`、`bsxscript`、`dsk`、`scf`、`scs`、`eme`、`asb`、`arc` |
| `brand/` | 按社（游戏厂商）分组：一个社一个目录（如 `brand/bishop/`、`brand/emonengine/`），社目录内为该社的格式包 | 无导出 |
| `brand/bishop/scriptbin/` | BSXScript / BSScript 共享容器实现（内部库，未注册为 feature） | `Kind`（`Modern`/`Classic`/`Ancient`）、`Layout`、`LayoutOf`、`LayoutForWords`、`DetectSignature`、`Decode`、`Container`；表名 `TableNames`、表 kind `TableKinds` |
| `brand/bishop/bsxscript/` | BSXScript 编解码 feature | `FeatureID`（`bishop.bsxscript`）、`FeatureName`（`Bishop-BSXScript`） |
| `brand/bishop/bsscript/` | BSScript 编解码 feature | `FeatureID`（`bishop.bsscript`）、`FeatureName`（`Bishop-BSScript`） |
| `brand/bishop/bdt/` | BDT（XOR 0xFF 文本）编解码 feature | `FeatureID`（`bishop.bdt`）、`FeatureName`（`Bishop-Bdt`）、`FormatName`（`bdt`） |
| `brand/bishop/scf/` | SCF 容器编解码 feature；`bishop.dsk` 通过 `Decode` / `EncodeParts` 复用其条目编解码 | `FeatureID`（`bishop.scf`）、`FeatureName`（`Bishop-Scf`）、`FormatName`（`SCF`）；`Decode`、`Encode`、`EncodeParts`、`LooksLikeSCF` |
| `brand/bishop/dsk/` | DSK+PFT 归档编解码 feature（内部调用 `scf`） | `FeatureID`（`bishop.dsk`）、`FeatureName`（`Bishop-Dsk`）、`FormatName`（`DSK`）；`DecodeWithPFT`、`EncodeWithPFT`、`FindPFT` |
| `brand/bishop/scs/` | SCS（异或加密记录流）编解码 feature | `FeatureID`（`bishop.scs`）、`FormatName`（`scs`）、`Codec` |
| `brand/emonengine/eme/` | EME 归档（EmonEngine 容器：索引记录加密 + 脚本条目 LZSS 压缩）编解码 feature | `FeatureID`（`emonengine.eme`）、`FeatureName`（`EmonEngine-Eme`）、`FormatName`（`eme`） |
| `brand/overdose/asb/` | Over-Dose Moon 的 ASB 剧本编解码 feature：16 字节容器头 + dword 掩码 + zlib，解码对白/选项，重编时重算指令 size 与跳转 field，文本按 UTF-8 写入（原版 CP932 读取仍兼容） | `FeatureID`（`overdose.asb`）、`FeatureName`（`Over-Dose ASB`） |
| `brand/overdose/arc/` | 直接吃 `script.arc` 的归档编解码 feature：解密头/索引/条目，把每个 `.asb` 当子文档合并成一份 Document；编码时重算条目偏移/大小、索引压缩与加密、最终文件大小种子 | `FeatureID`（`overdose.arc`）、`FeatureName`（`Over-Dose Script ARC`） |
| `translator/` | 翻译流水线：分块、并发翻译、残留日文重译、最终通检、回写 | `Runner`（`Run`）、`Request`、`Params`、`ProviderSpec`、`Event`；`ProviderFactory`（可替换变量，用于测试注入） |
| `translator/providers/` | OpenAI / Claude / Google 三种 API 协议实现（`net/http`，指数退避重试） | `Provider`（`Chat`）、`Spec`、`Message`、`Request`、`Response`；`New`、`Detect` |

## 构建与测试

前置：Host 构建（`-buildmode=c-shared` 依赖 cgo）需要 gcc；Android 交叉编译需要 NDK 中的
`aarch64-linux-android23-clang`。NDK 定位顺序与 CI 一致：
`ANDROID_NDK_LATEST_HOME` → `ANDROID_NDK_ROOT` → `ANDROID_HOME/ndk/*` 中版本排序最末者。

```bash
cd kernel
make host      # gcc 构建 libkernel.so（本机）
make android   # NDK clang 交叉编译 libkernel-android-arm64.so（arm64）
./scripts/build-android.sh   # 同上（CI 同款逻辑），产物为 libkernel.so，并复制到 app/android/app/src/main/jniLibs/arm64-v8a/libkernel.so
make test      # go test ./...
go vet ./... && gofmt -l .
```

样例测试：各格式包的往返测试遍历环境变量 `KZ_SAMPLES` 下的真实样例，
默认值为 `/root/code/Bishop/示例`；目录或样例缺失时测试跳过（`t.Skip`），不视为失败。
`go test ./...` 全量通过为合并前置条件。

约束：Go 侧只允许标准库，不引入第三方依赖（无 `go.sum` 依赖项）。

## C ABI

导出符号（`cmd/kernel/main.go`，`package main`）：

```go
//export KzInvoke
func KzInvoke(request *C.char) *C.char
//export KzVersion
func KzVersion() *C.char
//export KzFree
func KzFree(ptr *C.char)
```

- 入参与返回值均为 NUL 结尾的 UTF-8 JSON 字符串。
- 返回值由 Go 侧 `C.CString` 分配，调用方必须以 `KzFree`（内部 `C.free`）释放；
  `KzInvoke` 与 `KzVersion` 的返回值均适用此规则。
- `KzInvoke` 整体 `defer recover()`，任何 panic 转为 `{"ok":false,"error":"..."}`，
  不会穿越 C 边界；`request == nil` 时返回 `{"ok":false,"error":"nil request"}`。
- `KzVersion` 返回 `kizuna-kernel/1.0.0`，Flutter 侧用其做握手。
- 并发安全：除翻译任务中心（互斥锁保护）外无共享可变状态。

## JSON-RPC 协议

请求为 `{"op":"...", ...}`，响应统一为 `{"ok":true, ...}` 或 `{"ok":false,"error":"message"}`。

| op | 请求字段 | 响应字段 |
| --- | --- | --- |
| `ping` | 无 | `version`、`features[]` |
| `features` | 无 | `features[]` |
| `detect` | `path` | `detect`（`core.DetectResult` 的 JSON，见下） |
| `process` | `feature`、`action`、`input`、`output`，可选 `encoding`、`extra` | `result`（`core.Result` 的 JSON，见下） |
| `translate.detect` | `path` | `formats[]`：`{"featureId","format","name"}`（按 feature+format 去重） |
| `translate.start` | `source`、`target`、`feature`、`sourceEncoding`、`targetEncoding`、`provider`、`params`（见下）；`target` 为空时默认为 `source + ".out"` | `taskId`（int64，自增） |
| `translate.poll` | `taskId` | `done`（bool）、`events[]`（排空式：返回自上次 poll 以来累计的事件并清空；无新事件时为空数组；`done=true` 后仍可再 poll，5 分钟后任务回收；未知 taskId 返回错误） |
| `translate.cancel` | `taskId` | `canceled`（bool；已完成或未知 taskId 时为 false；取消成功后事件流追加 `{"type":"log","level":"warn","message":"已取消"}`） |

`features[]` 元素：`{"id","name","target","handlers"}`，其中 `target` 线值为 `"file"` /
`"dir"`，`handlers[]` 元素为 `{"action","label","suffixes","outputSuffix"}`。

`detect` 对象字段：`inputPath`、`inputKind`（`"file"` / `"dir"`）、`suffix`（命中的后缀，小写；
目录为 `.dir` 内层 stem 的扩展名）、`target`（`"file"` / `"dir"`）、`outputPath`
（首个候选的输出路径）、`candidates[]`、`message`（未识别时为 `未识别的文件类型` /
`未识别的目录类型`）。

`candidate` 元素字段：`featureId`、`featureName`、`action`、`actionLabel`、`label`
（`featureName + actionLabel`，如 `Bishop-Bdt解码`）、`format`、`inputPath`、`outputPath`。
文件探测按后缀长度降序排列命中（长后缀优先，如 `.dat.json` 优先于 `.dat`），
每个命中的 handler 再经 `DetectFormat`（读文件头 / 解析 JSON 的 `feature` 字段）甄别；
目录探测要求基名以 `.dir` 结尾（大小写不敏感），候选输出为去掉 `.dir` 的同级路径。

`result` 对象字段：`inputPath`、`outputPath`、`bytesIn`、`bytesOut`、`elapsedMs`、
`summary`、`details`。

`translate.start` 的 `provider` 对象：`kind`（`openai` / `google` / `claude`，为空或 `auto`
时按 baseUrl 自动判定）、`baseUrl`、`model`、`apiKey`、`timeoutSeconds`（缺省 120）、
`maxRetries`（缺省 3）。`params` 对象：`concurrency`（缺省 4，上限 32）、`chunkSize`
（缺省 30，上限 200）、`japaneseRetryLimit`（缺省 0，上限 10）、`targetLanguage`
（缺省 `简体中文`）、`temperature`、`glossary`（`原文=译名` 每行一条）、`contextLines`
（缺省 2，上限 10）。

`events[]` 事件类型与字段：

| `type` | 字段 |
| --- | --- |
| `log` | `level`（`info` / `warn`）、`message` |
| `progress` | `current`、`total`、`percent`（每完成一块发送一次） |
| `stats` | `promptTokens`、`completionTokens`、`requests`、`failed`、`elapsedMs`、`unitsPerSecond`（每秒至多一次，结束时再发送一次） |
| `residual` | `count`、`round`（最终通检前后） |
| `error` | `message` |
| `done` | `ok`（bool）、`outputPath`、`translated`、`skipped`、`elapsedMs` |

## 文档模型（解码产物）

解码输出的 JSON 即 `core.Document` 的序列化（`json.MarshalIndent`，2 空格缩进，末尾换行，
经 `AtomicWrite` 先写 `.tmp` 再改名落盘，权限 `0o644`）：

- `Document`：`feature`、`format`、`version`、`encoding`（解码时检测到的编码）、
  `units`、`sections`、`meta`（`values` / `lists` 键值与列表）。
- `Unit`：`id`、`kind`、`text`，可选 `speaker`、`name`、`prefix`、`suffix`
 （`prefix` / `suffix` 为原文中不可翻译的前后缀，翻译前后拼回，不送模型）。
- `Units` 是唯一的文本载体：`Section` 只存放 `IDs` 引用，不重复存放文本；翻译只改写
  `Units[*].Text`。
- `Unit.ID` 全局唯一，约定为 `<表>/<序号>`（如 `messages/12`、`line/451`）或
  SCF 的 `<条目>pool/<序号>` / `<条目>code/<序号>`。
- `Unit.Kind` 取值：`message`、`name`、`character`、`variable`、`property`、`label`、
  `choice`、`line`、`text`、`voice`，以及不可翻译的 `raw`（翻译流水线跳过 `raw`；
  命令关键字、素材名等不进入翻译）。

`Section.Kind` 语义（编码器按顺序拼接 section 重建文件）：

| Kind | 含义 | 关键字段 |
| --- | --- | --- |
| `bytes` | 原样字节（经 Base64 存放），编码时直接拼回 | `Base64`、`Size` |
| `values` | 连续 int32 字段表（SCF 头、条目表、池偏移表、跳转表等）；编码时重算其中的偏移 / 长度 | `Values` |
| `strings` | 字符串池 + 偏移表（脚本符号表、SCF 池） | `Encoding`（`utf16le` / `utf8` / `cp932` / `gbk`）、`UnitSize`（utf16le 为 2，其余为 1）、`IDs`（顺序即偏移表顺序）、`Trailing`（SCF 池 sidecar，Base64） |
| `lines` | 行文本（BDT、SCS） | `Encoding`、`EOL`（`crlf` / `lf`，BDT 另以 `Meta.Lists["eol"]` 逐行记录、`Trailing` 记录末行换行）、`IDs` |
| `code` | 指令区原样字节（BSXScript / BSScript 代码段、SCF 代码段）；指令中的消息引用均为表下标，文本长度变化不影响 code | `Base64`，SCF 另带 `Encoding`、`IDs`、`Trailing`（code sidecar，Base64） |

统一要求：`Encode(Decode(x)) == x`（逐字节相等）；无法结构化重算的区间退化为 `bytes`
原样保存，但文本必须进入 `strings` / `lines`。

编码策略：编码目标由 `doc.Encoding` 决定（SCF 为逐 section 的 `Encoding`，
`docEncodingOverride` 仅在与主导编码不一致时整体覆盖；SCS 的 `process` 支持
`encoding` 参数覆盖）。目标编码无法表示译文字符时回落为 UTF-8：
BDT 在 `Meta.Values["encoding.fallback"]` 记录 `<原编码>->utf8` 并改写 `doc.Encoding` /
section 编码为 `utf8`；SCS 按记录逐条回落；SCF 按文本逐条回落（`0xFF` 为填充保留字节，
含其文本拒绝编码）。

## 已注册 feature

| ID | 显示名 | 方向 | 输入后缀 | 输出后缀 | 格式识别规则 | 样例往返覆盖率 |
| --- | --- | --- | --- | --- | --- | --- |
| `bishop.bsxscript` | `Bishop-BSXScript` | 解码 / 编码 | `.dat` / `.dat.json` | `.dat.json` / 去后缀 | 解码：16 字节签名以 `BSXScript` 开头（含 `1.0` / `2.0` / `3.0` / `3.1` / `3.2` / `3.3`，支持 XOR 混淆头）；编码：JSON 的 `feature` 字段归属判定 | 45/45（1.0×2、2.0×4、3.0×6、3.1×9、3.2×9、3.3×15） |
| `bishop.bsscript` | `Bishop-BSScript` | 解码 / 编码 | `.dat` / `.dat.json` | `.dat.json` / 去后缀 | 解码：签名以 `BSScript` 开头且非 `BSXScript`（含无版本号 Ancient 与 `Rev.2`~`Rev.6`，支持 XOR 混淆头）；编码：`feature` 归属判定 | 12/12 |
| `bishop.bdt` | `Bishop-Bdt` | 解码 / 编码 | `.bdt` / `.bdt.json` | `.bdt.json` / 去后缀 | 以后缀为准，无魔数校验（整文件 XOR 0xFF 后为 CP932 CRLF 文本）；编码：`feature` 归属判定 | 2/2（`scene00.bdt` 47842 units、`scene00(1).bdt` 47472 units） |
| `bishop.scs` | `Bishop-Scs` | 解码 / 编码 | `.scs` / `.scs.json` | `.scs.json` / 去后缀 | 以后缀为准；加密分支按 `seed = 记录起始偏移 \| 1`、`seed *= 69069` 低位异或解密，明文分支（按行切分）兼容；编码：`feature` 归属判定 | 2/2（`MAINCON.SCS` 76477 units、`MAINCON (1).SCS` 40537 units） |
| `bishop.scf` | `Bishop-Scf` | 解码 / 编码 | `.scf` / `.scf.json` | `.scf.json` / 去后缀 | 解码：`LooksLikeSCF` 结构校验（10 字节头 + 区段边界）；编码：`feature` 归属判定 | 4620/4620 条目（含 SERAPH 方言 128/128，TJ2 方言 `SCENE` 1207 + `scene(1)` 3285 条目往返一致） |
| `bishop.dsk` | `Bishop-Dsk` | 解码 / 编码 | `.dsk` / `.dsk.json` | `.dsk.json` / 去后缀 | `.dsk` 以后缀为准；解码时按同名 `.pft`（`FindPFT`：先小写后大写）定位索引；PFT 头为 u16 headerSize + u16 clusterSize + i32 count，条目 16 字节（8 字节名 + u32 cluster + u32 size，偏移 = cluster × clusterSize）；编码：`feature` 归属判定 | 3 档 DSK+PFT 逐字节一致（`SCENE` 1207 条目 / 14959 units、`SCNFILE(2)` 128 条目 / 1797 units、`scene(1)` 3285 条目 / 122485 units） |
| `emonengine.eme` | `EmonEngine-Eme` | 解码 / 编码 | `.eme` / `.eme.json` | `.eme.json` / 去后缀 | 解码：8 字节魔数 `RREDATA ` 命中；脚本条目（subType 3）按 12 字节加密头（packed/part2）+ LZSS 解压后逐条检测编码（BOM 优先再 `DetectTextEncoding`）；条目名按 CP932 解码；编码：`feature` 归属判定，未改动时原样返回源文件字节，改动条目按 CP932（失败回落为带 BOM 的 UTF-8）+ LZSS 重压缩回写并重算偏移 | 15/15 档案逐字节一致（1937 条目，其中 1274 脚本条目 / 526522 units） |

`Document.format` 取值：bsxscript / bsscript 为签名串（如 `BSXScript 3.3`、`BSScript Rev.6`），
其余为 `bdt` / `scs` / `SCF` / `DSK` / `eme`。

## 扩展指南

### 新增一个"社"（顶层格式家族）

1. 在 `brand/` 下新建社目录（如 `brand/newsoc/`），社目录内新建格式包目录并实现 `core.Feature`
  （文件类格式同时实现 `core.Codec` 的 `Decode` / `Encode` / `UnitText` / `SetUnitText`，
   后两者直接包 `core.DefaultUnitText` / `core.DefaultSetUnitText`）。
2. 包内 `init()` 调用 `core.Register`。
3. 在 `drivers/drivers.go` 增加对应空导入。
4. 该"社"内部的共享解析逻辑放入同级内部包（如 `scriptbin` 之于 bsxscript / bsscript），
   共享包不注册 feature，只被格式包导入（避免循环导入：被依赖方不得反向依赖）。

### 新增一个文件格式 feature

1. 实现 `core.Codec`：`ID`（点分，如 `bishop.newfmt`）、`Name`（显示名）、`Target()`
   （文件类返回 `core.TargetFile`）、`Handlers()`（解码 handler 后缀为原文后缀、
   `OutputSuffix` 为原文后缀加 `.json`；编码 handler 后缀为解码输出后缀、
   `OutputSuffix` 为空表示去掉该后缀）、`DetectFormat`、`Run`（按 `action` 分发到
   `core.RunCodecDecode` / `core.RunCodecEncode`）、`Decode` / `Encode` /
   `UnitText` / `SetUnitText`。
2. `init()` 注册；在 `drivers/drivers.go` 加空导入。
3. `DetectFormat` 约定：解码方向读文件头做魔数 / 结构判定（无魔数格式直接返回 true）；
   编码方向读入 JSON 解析 `feature` 字段（可用 `core.ProbeFeature`），非本 feature
   返回 `("", false)`，`Encode` 内非本 feature 返回 `core.ErrNotMine`。
4. 编写往返测试：遍历 `KZ_SAMPLES` 下本格式全部样例做 `Decode→Encode` 逐字节比对，
   另加一条改文本后重编码用例；样例缺失时 `t.Skip`。
5. `Handlers()` 后缀约定：解码输出固定为输入路径追加 `.json`（如 `.dat` →
   `.dat.json`）；编码输出固定为去掉命中的输入后缀（`OutputSuffix == ""`）。

### 新增一个目录类（打包 / 解包）feature

1. `Target()` 返回 `core.TargetDirectory`；`Handlers()` 提供 `pack`（后缀如 `.bsa`）
   与 `unpack`（后缀 `.bsa`、`.pk` 等）两个 handler。
2. 目录探测约定（`core.detectDirectory`）：输入基名以 `.dir` 结尾（大小写不敏感），
   `stem` 为去 `.dir` 后的基名，`suffix` 为 stem 的小写扩展名（如 `bgm.bsa.dir` →
   stem `bgm.bsa`、suffix `.bsa`）；handler 的 `Suffixes` 含该 suffix 即成为候选，
   输出为去掉 `.dir` 的同级路径。
3. 当前无已注册的目录类 feature 可供参照。

### 翻译流水线接入点

- `translator.Runner{}.Run(ctx, req, emit)`：流水线总入口。流程为
  `core.Find(feature)` 取 codec → `Decode` → 收集可翻译单元（`Kind != "raw"` 且含日文）
  → 按 `chunkSize` 分块（同块注入命中的术语）→ `concurrency` 个 worker 并发请求模型
  （行号回填 `[i] 译文`，缺失行号单独重试）→ 残留日文按 `japaneseRetryLimit` 单条重译 →
  最终通检（残留条目附原文与 `contextLines` 上下文复核，仍残留则记 warn 保留译文）→
  `SetUnitText` 回写 → `Encode` → `AtomicWrite(target)`。
- `shell.TranslationRunner`：`Runner` 的函数变量，生产环境指向
  `translator.Runner{}.Run`，测试时替换为桩实现。
- `translator.ProviderFactory`：provider 构造变量，默认 `providers.New`，测试时替换。
- `sourceEncoding` 为空或 `auto` 时采用解码所得编码；`targetEncoding` 为空或 `auto`
  时保持源编码，否则覆盖 `doc.Encoding`（DSK 除外，见下节）后编码输出。

## 已知限制与注意点

- SCF 存在 SERAPH 与字节 / 字两种非 SERAPH（TJ2）方言分支：`!` / `#` / `[` / `(`
  操作数宽度按分支解释；分支判定优先走 SERAPH，仅当字节或字宽度的 `!s` 前缀序列
  满足启发式（取值 0~112294 且互不重复）时采用非 SERAPH 分支。
- 翻译输出的编码策略：`targetEncoding=utf8` 产出的文件仅 KizunaEngine 自身可回读，
  原版程序按 CP932 解析会显示异常。
- DSK 条目重编码不响应顶层 `doc.Encoding`：`EncodeWithPFT` 经 `scf.EncodeParts`
  以各 section 自带编码重建 SCF 条目，翻译的 `targetEncoding` 对 DSK 内 SCF 文本不生效。
- 当前文件处理页的调用固定不传 `encoding` / `extra` 参数：
  协议支持但 Dart 侧固定不传；当前仅 SCS 的 `Run` 会读取 `encoding` 覆盖。
- `libkernel.so` 由 CI 构建，不提交版本库
  （见仓库根 `.gitignore`）；换功能时替换随 APK 打包的
  `app/android/app/src/main/jniLibs/arm64-v8a/libkernel.so` 即可。
