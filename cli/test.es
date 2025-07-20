package main

message := "Hello World"

fn main() {
    printer := test()
    printer()
}

fn test() {
    return fn() {
        echo message
    }
}