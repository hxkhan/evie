package main

message := "Hello World"

fn main() {
    // test 1 layer of capture
    printer := single()
    printer()

    // test 2 layers of capture
    getter := double()
    printer = getter()
    printer()
}

fn single() {
    // printer
    return fn() {
        echo message
    }
}

fn double() {
    // getter
    return fn() {
        // printer
        return fn() {
            echo message
        }
    }
}