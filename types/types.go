package types

import "strings"

// Kind is the discriminator - tells you which concrete Type you're holding
type Kind int

const (
	KindNil Kind = iota
	KindInt
	KindFloat
	KindString
	KindBool
	KindStruct
	KindFn
	KindGeneric // List[T], Map[K,V], etc.
	KindUnion   // str | bool, int | nil, etc.  <-- add this
)

// Type is the core interface that every type in the system implements
type Type interface {
	Kind() Kind
	String() string
	Equals(other Type) bool
}

// PrimitiveType covers void, int, float, string, bool, nil
// There's one singleton for each, no need to allocate
type PrimitiveType struct{ kind Kind }

func (p *PrimitiveType) Kind() Kind         { return p.kind }
func (p *PrimitiveType) String() string     { return kindName[p.kind] }
func (p *PrimitiveType) Equals(o Type) bool { return p.kind == o.Kind() }

// Singletons - always compare by pointer or Kind, never allocate new ones
var (
	Int    Type = &PrimitiveType{KindInt}
	Float  Type = &PrimitiveType{KindFloat}
	String Type = &PrimitiveType{KindString}
	Bool   Type = &PrimitiveType{KindBool}
	Nil    Type = &PrimitiveType{KindNil}
)

var kindName = map[Kind]string{
	KindInt: "int", KindFloat: "float",
	KindString: "string", KindBool: "bool", KindNil: "nil",
}

// --- Struct ---

type StructType struct {
	Name   string
	Fields []Field
}

type Field struct {
	Name string
	Type Type
}

func (s *StructType) Kind() Kind     { return KindStruct }
func (s *StructType) String() string { return s.Name }
func (s *StructType) Equals(o Type) bool {
	// Nominal equality - same name means same type
	// If you want structural equality, compare fields instead
	other, ok := o.(*StructType)
	return ok && s.Name == other.Name
}

func (s *StructType) FieldType(name string) (Type, bool) {
	for _, f := range s.Fields {
		if f.Name == name {
			return f.Type, true
		}
	}
	return nil, false
}

// --- Function ---

type FnType struct {
	Params []Type
	Return Type
}

func (f *FnType) Kind() Kind { return KindFn }
func (f *FnType) Equals(o Type) bool {
	other, ok := o.(*FnType)
	if !ok || len(f.Params) != len(other.Params) {
		return false
	}
	for i, p := range f.Params {
		if !p.Equals(other.Params[i]) {
			return false
		}
	}
	return f.Return.Equals(other.Return)
}
func (f *FnType) String() string {
	b := strings.Builder{}
	b.WriteString("fn(")
	for i, p := range f.Params {
		b.WriteString(p.String())
		if i != len(f.Params)-1 {
			b.WriteString(", ")
		}
	}
	b.WriteString("): ")
	if f.Return != nil {
		b.WriteString(f.Return.String())
	} else {
		b.WriteString("void")
	}
	return b.String()
}

// --- Generic (List[T], Map[K,V], etc.) ---

type GenericType struct {
	Name   string // "List", "Map", etc.
	Params []Type // the type arguments
}

func (g *GenericType) Kind() Kind { return KindGeneric }
func (g *GenericType) String() string {
	b := strings.Builder{}
	b.WriteString(g.Name)
	b.WriteByte('[')
	for i, p := range g.Params {
		b.WriteString(p.String())
		if i != len(g.Params)-1 {
			b.WriteString(", ")
		}
	}
	b.WriteByte(']')
	return b.String()
}
func (g *GenericType) Equals(o Type) bool {
	other, ok := o.(*GenericType)
	if !ok || g.Name != other.Name || len(g.Params) != len(other.Params) {
		return false
	}
	for i, p := range g.Params {
		if !p.Equals(other.Params[i]) {
			return false
		}
	}
	return true
}

// --- Union ---
type UnionType struct {
	Members []Type // deduplicated, canonical order not guaranteed
}

func NewUnion(members ...Type) Type {
	// Flatten nested unions and deduplicate
	seen := make([]Type, 0, len(members))
	for _, m := range members {
		if u, ok := m.(*UnionType); ok {
			for _, inner := range u.Members {
				if !containsType(seen, inner) {
					seen = append(seen, inner)
				}
			}
		} else {
			if !containsType(seen, m) {
				seen = append(seen, m)
			}
		}
	}
	// A union of one is just that type
	if len(seen) == 1 {
		return seen[0]
	}
	return &UnionType{Members: seen}
}

func containsType(ts []Type, t Type) bool {
	for _, existing := range ts {
		if existing.Equals(t) {
			return true
		}
	}
	return false
}

func (u *UnionType) Kind() Kind { return KindUnion }

func (u *UnionType) String() string {
	b := strings.Builder{}
	for i, m := range u.Members {
		b.WriteString(m.String())
		if i != len(u.Members)-1 {
			b.WriteString(" | ")
		}
	}
	return b.String()
}

// Equals is order-insensitive: str|bool == bool|str.
// Both unions must have the same size and every member of each
// must appear (by Equals) in the other.
func (u *UnionType) Equals(o Type) bool {
	other, ok := o.(*UnionType)
	if !ok || len(u.Members) != len(other.Members) {
		return false
	}
	for _, m := range u.Members {
		if !containsType(other.Members, m) {
			return false
		}
	}
	return true
}

func (u *UnionType) Contains(t Type) bool {
	return containsType(u.Members, t)
}
