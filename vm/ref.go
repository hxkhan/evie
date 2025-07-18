package vm

type reference struct {
	index  int
	scroll int
}

func (ref reference) isBuiltin() bool {
	return ref.scroll < 0
}

func (ref reference) isLocal() bool {
	return ref.scroll == 0
}

func (ref reference) isCaptured() bool {
	return ref.scroll > 0
}
