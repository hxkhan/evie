package main

fn main() {
    timer := time.timer(500, true)

    await timer
    echo "tick"

    /* await timer
    echo "tick"

    await timer
    echo "tick"

    await timer
    echo "tick"

    await timer
    echo "tick" */

    await time.timer(1000, false)
    await timer
    echo "tick"


    //discontinue timer
}