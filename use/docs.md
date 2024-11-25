# EvieScript Reference

## Concurrency
```
package main

fn test(message) {
    time.sleep(1000)
    echo message
}

fn main() {
    go test("one")
    go test("two")
    go test("three")
    go test("four")
    go test("five")
    go test("six")
    go test("seven")
    go test("eight")
    go test("nine")
    go test("ten")

    echo "before"
    time.sleep(2000)
    echo "after"
}
```

```
package main

fn test(message, duration) {
    time.sleep(duration)
    echo message
}

fn main() {
    go test("one", 100)
    go test("two", 200)
    go test("three", 300)
    go test("four", 400)
    go test("five", 500)
    go test("six", 600)
    go test("seven", 700)
    go test("eight", 800)
    go test("nine", 900)
    go test("ten", 1000)

    echo "before"
    time.sleep(2000)
    echo "after"
}
```