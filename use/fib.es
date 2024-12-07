package main

calls := 0

fn main() {
    echo fib(35)
    echo calls // 29 860 703
}

fn fib(n) {
    calls += 1
    if (n < 2) return n
    
    return fib(n-1) + fib(n-2)
}