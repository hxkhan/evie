package main

message := "hello world"

fn main() {
    echo message.split(" ").join("_")
    echo join(split(message, " "), "_")


    echo message.split(" ")
    echo split(message, " ")
}