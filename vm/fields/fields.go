package fields

type ID int

var registryA = map[string]ID{}
var registryB = map[ID]string{}

func Get(name string) ID {
	index, exists := registryA[name]
	if !exists {
		id := ID(len(registryA))
		registryA[name] = id
		registryB[id] = name
		return id
	}
	return index
}

func Lookup(id ID) string {
	return registryB[id]
}
