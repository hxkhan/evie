package vm

import "github.com/hxkhan/evie/vm/fields"

func NewHostPackage() Package {
	return &packageInstance{
		globals: map[fields.ID]Global{},
	}
}

type Package interface {
	SetSymbol(name string, value Value) (existed bool) // sets a global symbol
	HasSymbol(name string) (exists bool)               // checks if a symbol exists
	GetSymbol(name string) (sym Global, exists bool)   // does a symbol lookup
	Box() (value Value)                                // boxes an evie package to be used as a value
}
