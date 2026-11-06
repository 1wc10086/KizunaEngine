package scf

const ScfSignatureValue = "SCF"

func IsScf(data []byte) bool {
	header, ok := ReadScfHeader(data)
	if !ok {
		return false
	}
	return header.IsTransferCountValid() && header.CodeLength > 0 && header.TotalLength() == len(data)
}
