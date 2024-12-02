package time

/* func Export() {
	std.ImportFn(timer)
}

func timer(duration core.Value) (core.Value, error) {
	if duration, ok := duration.AsInt64(); ok {
		return core.NewTask(func() (core.Value, error) {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return nil, nil
		})
	}
	return nil, core.ErrTypes
} */
