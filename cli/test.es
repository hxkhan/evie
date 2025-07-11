package main

fn main() {
    z := 1

    do(500000, fn() {
        z = z + 1
    })

    echo z
}

fn do(n, callback) {
    if (n > 0) {
        n = n - 1
        callback()
        do(n, callback)
    }
}