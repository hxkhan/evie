package main

fn pow(x, n) {
    if (n == 0) return 1
    
    return x * pow(x, n-1)
}

fn clamp(value, min, max) {
    if (value < min) 
        return min
    else if (value > max)
        return max

    return value
}

fn factorial(n) {
    if (n < 2) return 1

    return n * factorial(n-1)
}

fn main() {
    echo pow(2, 3)
    echo clamp(50, 0, 100)
    echo clamp(50, 60, 100)
    echo clamp(50, 0, 40)
    echo factorial(5)
}