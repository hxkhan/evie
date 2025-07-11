package main

fn print(message, duration) {
    await time.timer(duration)
    echo message
}

fn main() {
    go print("one", 100)
    go print("two", 200)
    go print("three", 300)
    go print("four", 400)
    go print("five", 500)
    go print("six", 600)
    go print("seven", 700)
    go print("eight", 800)
    go print("nine", 900)
    go print("ten", 1000)
}


/* fn main() {
    go print("one", 100)

    echo "before"
    await time.timer(2000)
    echo "after"
}

fn print(message, duration) {
    await time.timer(duration)
    echo message
} */