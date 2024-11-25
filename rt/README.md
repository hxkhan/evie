IGNORE THIS FOLDER MOSTLY, only 5% of it is used 

## Concurrency
All builtin blocking calls run with the GIL released!

### Source Language
```rust
fn test() {
	print("Hello from the 'test' function!")
}

go test()

println("Going to sleep!")
sleep(3000)
println("Back from sleeping!")
```

### Go transpilation
```go
var test any
test = func() any {
	defer rt.ExitFn("test")

	println("Hello from the 'test' function!")
	return nil
}

go func() {
	rt.GIL.Lock()
	rt.FnAs[func() any]("test", test)()
	rt.GIL.Unlock()
}()

println("Going to sleep!")
rt.FnAs[func(any) any]("sleep", sleep)(rt.Int(3000))
println("Back from sleeping!")
```

<p style="page-break-before: always">

## Errors

### Source Language
```rust
fn handler(data) {
	var body = try json_decode(data) {
		// variable 'error' will be implicitly declared
		print("Error occured:", error)
		return anything
	}

	// success, body will contain result
	print("Success!")
	return anything
}
```

### Go transpilation
```go
var handler any
handler = func(data any) (_res_ any) {
	defer rt.ExitFn("handler")
	
	var body any = func() any {
		defer rt.Catch(func(error error) any {
			print("Error occured:", error)
			return anything
		}, &_res_)
		return json_decode(data)
	}()

	print("Success!")
	return anything
}
```

```go
var names = filter(people, fn(person) -> person["name"])


filter(numbers, fn(x) -> x > 10)

// resulting byte code
call 1
load filter
load numbers
fn "x"
ret
mr
load x
int 10
end

// start server
var server = net.listen("tcp", 8800)
var server = net.listen("udp", 8800)

// tcp waiting for client
var client = net.accept(server)
net.send(client, "Welcome!")

// tcp reading from client
var data = net.accept(client)

// udp waiting for data
var (client, data) = net.accept(server)
net.send(client, "Welcome!")

// connecting to host
var socket = net.establish("tcp", "212.234.545.312", 8800)
var socket = net.establish("udp", "212.234.545.312", 8800)
net.send(socket, "Hello Server!")

interface net {
	func establish(protocol, addr, port) handle
	func listen(protocol, port) handle
	func accept(handle) data | (client, data)
	func send(handle, data)
}
```