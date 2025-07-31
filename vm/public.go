package vm

func NewHostPackage(name string) Package {
	return &packageInstance{
		name:    name,
		globals: map[int]*Value{},
	}
}

type Package interface {
	SetSymbol(name string, value Value) (overridden bool) // sets a global symbol
	HasSymbol(name string) (exists bool)                  // checks if a symbol exists
	GetSymbol(name string) (sym Symbol, exists bool)      // does a symbol lookup
	Box() (value Value)                                   // boxes an evie package to be used as a value
}
