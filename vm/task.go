package vm

func NewTask(fn func() (Value, *Exception)) Value {
	task := make(chan evaluation, 1)
	go func() {
		res, err := fn()
		task <- evaluation{res, err}
		close(task)
	}()
	return BoxTask(task)
}

type evaluation struct {
	result Value
	err    *Exception
}
