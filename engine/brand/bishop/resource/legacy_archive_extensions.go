package resource

import "strings"

const (
	LegacyArchiveKgExtension  = ".KG"
	LegacyArchiveScfExtension = ".SCF"
	LegacyArchiveAdpExtension = ".ADP"
	LegacyArchiveBafExtension = ".BAF"
	LegacyArchiveThmExtension = ".THM"
	LegacyArchiveIvdExtension = ".IVD"
)

func LegacyArchiveExtensionForDsk(dskPath string) string {
	switch strings.ToUpper(pathFileNameWithoutExtension(dskPath)) {
	case "SCENE", "SCENARIO", "SCNFILE":
		return LegacyArchiveScfExtension
	case "VISUAL", "VISUAL1", "VISUAL2":
		return LegacyArchiveKgExtension
	case "SOUND", "PCM1", "PCM2", "PCM", "ADPCM", "EFCFILE", "PCMFILE", "BGMFILE":
		return LegacyArchiveAdpExtension
	case "WIPE":
		return LegacyArchiveBafExtension
	case "THUMB":
		return LegacyArchiveThmExtension
	case "IVENT":
		return LegacyArchiveIvdExtension
	default:
		return LegacyArchiveKgExtension
	}
}
