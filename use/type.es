package main

type person(id) {
    var hunger = 0.2
    var thirst = 0.3

    fn eat() {
        hunger = 0
    }

    fn drink() {
        thirst = 0
    }
}

var p1 = new person("0305250177")
var p1 = new person("0303216723")

echo p1.hunger