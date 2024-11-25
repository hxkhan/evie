package main

inc := null

fn test(x) {
    inc = fn() {
        x += 1
    }

    return fn() => echo x
}

fn main() {
    a := 10
    b := a

    b += 1

    echo a
    echo b

    printer := test(200)
    printer()
    inc()
    printer()
    inc()
    printer()
}