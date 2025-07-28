package vm

type Package interface {
	HasSymbol(name string) (exists bool)             // checks if a symbol exists
	GetSymbol(name string) (sym Symbol, exists bool) // does a symbol lookup
	Box() (value Value)                              // boxes an evie package to be used as a value
	//GetElseCreateSymbol() (sym Symbol)
}
