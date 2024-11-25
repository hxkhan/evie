package time

/* func Export() {
	std.ImportFn(timer)
}

func timer(duration box.Value) (box.Value, error) {
	if duration, ok := duration.AsInt64(); ok {
		return core.NewTask(func() (box.Value, error) {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return nil, nil
		})
	}
	return nil, core.ErrTypes
} */
