calls = 0


def fib(n):
    global calls
    calls += 1

    if n < 2:
        return n

    return fib(n-1) + fib(n-2)

print(fib(35))
print(calls)