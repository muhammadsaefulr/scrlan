package scanner

func NewScanner() (Scanner, error) {
	return newPacketScanner(), nil
}
