package gfx

type BsgCompression byte

const (
	BsgCompressionNone BsgCompression = 0
	BsgCompressionRle  BsgCompression = 1
	BsgCompressionLz   BsgCompression = 2
)
