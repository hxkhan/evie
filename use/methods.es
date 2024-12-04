package main

fn main() {
    message := "hello world"

    echo message.split(" ")
    echo split(message, " ")

    echo message.split(" ").join("_")
    echo join(split(message, " "), "_")
}