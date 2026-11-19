package text

import "strings"

func containsFold(token string, value string) bool {
	return strings.Contains(strings.ToLower(token), strings.ToLower(value))
}

func ParseSpriteKind(token string) EmonSpriteKind {
	if len(token) == 0 {
		return EmonSpriteKindNone
	}

	kind := EmonSpriteKindNone
	if containsFold(token, "GROUP") {
		kind |= EmonSpriteKindGroup
	}

	if containsFold(token, "BACK") {
		kind |= EmonSpriteKindBack
	}

	if containsFold(token, "MIDDLE_F") {
		kind |= EmonSpriteKindMiddleFront
	}

	if containsFold(token, "MIDDLE_B") {
		kind |= EmonSpriteKindMiddleBack
	}

	return kind
}

func ParseSpriteDisplayFlags(token string) EmonSpriteFlags {
	if len(token) == 0 {
		return EmonSpriteFlagNone
	}

	flags := EmonSpriteFlagNone
	if containsFold(token, "DISPON") {
		flags = EmonSpriteFlagVisible
	}

	if containsFold(token, "CLIP") {
		flags |= EmonSpriteFlagClip
	}

	if containsFold(token, "SELECT") {
		flags |= EmonSpriteFlagSelect
	}

	if containsFold(token, "EFFECTONLY") {
		flags |= EmonSpriteFlagEffectOnly
	}

	if containsFold(token, "DRAWME") {
		flags |= EmonSpriteFlagDrawMe
	}

	if containsFold(token, "CLEARME") {
		flags |= EmonSpriteFlagClearMe
	}

	if containsFold(token, "BACK") {
		flags |= EmonSpriteFlagBack
	}

	if containsFold(token, "TILE") {
		flags |= EmonSpriteFlagTile | EmonSpriteFlagClip
	}

	return flags
}

func ParseSpriteResourceFlags(token string) int {
	if len(token) == 0 {
		return 6
	}

	flags := 0
	if containsFold(token, "FILE") {
		flags = 4
	}

	if containsFold(token, "SPRITE") {
		flags |= 2
	}

	if containsFold(token, "NEW") {
		flags |= 1
	}

	if containsFold(token, "CLEARBLACK") {
		flags |= 0x200
	}

	if containsFold(token, "CLEARNULL") {
		flags |= 0x100
	}

	if containsFold(token, "MASK") {
		flags |= 0x1000
	}

	if containsFold(token, "TRANCE") {
		flags |= 0x2000
	}

	if containsFold(token, "NOTEREA") {
		flags |= 8
	}

	if containsFold(token, "COLOR") {
		flags |= 0x10
	}

	if containsFold(token, "MUST") {
		flags |= 0x1000000
	}

	if flags != 0 {
		return flags
	}

	return 6
}

func ParseSpriteSwitchFlags(token string) EmonSpriteFlags {
	if len(token) == 0 {
		return EmonSpriteFlagNone
	}

	flags := EmonSpriteFlagNone
	if containsFold(token, "DISPON") {
		flags |= EmonSpriteFlagVisible
	}

	if containsFold(token, "SELECTON") {
		flags |= EmonSpriteFlagSelect
	}

	if containsFold(token, "MTNON") {
		flags |= EmonSpriteFlagMotion
	}

	if containsFold(token, "ANMON") {
		flags |= EmonSpriteFlagAnimation
	}

	return flags
}
