package fields

type ID int

var registry = map[string]ID{}

func Get(name string) ID {
	index, exists := registry[name]
	if !exists {
		registry[name] = ID(len(registry))
		return ID(len(registry) - 1)
	}
	return index
}
