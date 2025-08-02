package fields

var registry = map[string]int{}

func Get(name string) int {
	index, exists := registry[name]
	if !exists {
		registry[name] = len(registry)
		return len(registry) - 1
	}
	return index
}
