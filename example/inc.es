package main

x := 0

fn inc(n) {
    if (n > 0) {
        x = x + 1
        inc(n-1)
    }
}

fn print() {
    echo x
}

fn main() {
    go inc(10000)
    go inc(10000)

    //await time.timer(10, false)

    //echo x
    go print()
}