package eme

type EmeEntry struct {
	Name         string
	SubType      int32
	Size         int32
	UnpackedSize int32
	Offset       int64
	FrameSize    int32
	FrameInitPos int32
}

func (e *EmeEntry) IsScript() bool {
	return e.SubType == 3
}

func (e *EmeEntry) IsImage() bool {
	return e.SubType == 4
}

func (e *EmeEntry) IsStored() bool {
	return e.FrameSize == 0
}
