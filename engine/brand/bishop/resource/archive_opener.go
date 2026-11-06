package resource

import "kizuna/engine/core/strutil"

func ArchiveIsSupported(path string) bool {
	return ArchiveIsSupportedWithIndex(path, nil)
}

// ArchiveIsSupportedWithIndex 与 ArchiveIsSupported 相同，但允许调用方传入
// “stem -> .Inx 路径”的全局索引。旧作多光盘安装把 .Snn 与 .Inx 分在不同目录，
// 只查同目录会把整个归档判成不存在。
func ArchiveIsSupportedWithIndex(path string, snnIndexes map[string]string) bool {
	switch {
	case strutil.HasSuffixFold(path, SnnExtension):
		return SnnIsArchiveWithIndex(path, snnIndexes)
	case strutil.HasSuffixFold(path, PackPkExtension):
		return PackPkIsArchive(path)
	case strutil.HasSuffixFold(path, DskExtension):
		return DskIsArchive(path)
	case strutil.HasSuffixFold(path, ZbmExtension):
		return ZbmIsArchive(path)
	default:
		return strutil.HasSuffixFold(path, ".bsa") && BsaIsArchive(path)
	}
}

func ArchiveOpen(path string) (Archive, error) {
	return ArchiveOpenWithIndex(path, nil)
}

func ArchiveOpenWithIndex(path string, snnIndexes map[string]string) (Archive, error) {
	switch {
	case strutil.HasSuffixFold(path, SnnExtension):
		return SnnOpenWithIndex(path, snnIndexes)
	case strutil.HasSuffixFold(path, PackPkExtension):
		return PackPkOpen(path)
	case strutil.HasSuffixFold(path, DskExtension):
		return DskOpen(path)
	case strutil.HasSuffixFold(path, ZbmExtension):
		return ZbmOpen(path)
	default:
		return BsaOpen(path)
	}
}
