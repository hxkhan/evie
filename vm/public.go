package vm

import "github.com/hxkhan/evie/vm/fields"

func NewHostPackage(name ...string) Package {
	pkgName := ""
	if len(name) > 0 {
		pkgName = name[0]
	}
	return &packageInstance{
		name:    pkgName,
		globals: map[fields.ID]Global{},
	}
}

type Package interface {
	Name() string                                      // name of the package
	SetName(name string)                               // sets package name
	SetSymbol(name string, value Value) (existed bool) // sets a global symbol
	HasSymbol(name string) (exists bool)               // checks if a symbol exists
	GetSymbol(name string) (sym Global, exists bool)   // does a symbol lookup
	Get(name string) (value Value, exists bool)        // gets a symbol's Value directly
	Box() (value Value)                                // boxes an evie package to be used as a value
}
