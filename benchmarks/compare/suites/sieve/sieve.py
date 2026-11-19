def sieve(n):
    flags = []
    for _ in range(n):
        flags.append(0)

    i = 2
    while i * i < n:
        if flags[i] == 0:
            j = i * i
            while j < n:
                flags[j] = 1
                j += i
        i += 1

    count = 0
    for k in range(2, n):
        if flags[k] == 0:
            count += 1
    return count

print(sieve(1000000))
