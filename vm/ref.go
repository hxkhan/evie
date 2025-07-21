package vm

// reference represents a lexical reference
type reference struct {
	scroll int
	index  int
}

func (ref reference) isBuiltin() bool {
	return ref.scroll < 0
}

func (ref reference) isLocal() bool {
	return ref.scroll == 0
}

func (ref reference) isCapture() bool {
	return ref.scroll > 0
}
