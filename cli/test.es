package main


fn main() {
    x := 10

    printer := test()
    printer()
}

message := "Hello World"

fn test() {
    msg := message
    return fn() {
        echo msg
    }
}


/* fn main() {
    x := 10

    printer := test()
    printer()
}

fn test() {
    msg := message
    return fn() {
        echo msg
    }
} */

/* 
fn test() {
    y := 20 // lives on stack temporarily, is popped after test() returns
    return fn() { // getter
        return fn() { // printer
            echo y
        }
    }
}

getter := test() // 
printer := go getter() // error: index 0 out of bounds, len(stack) = 0 
printer()
 */