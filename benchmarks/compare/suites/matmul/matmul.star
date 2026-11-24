def matmul(n):
    a = []
    b = []
    for i in range(n):
        ra = []
        rb = []
        for j in range(n):
            ra.append((i * 3 + j * 7) % 17)
            rb.append((i * 5 + j * 11) % 19)
        a.append(ra)
        b.append(rb)

    total = 0
    for i in range(n):
        ai = a[i]
        for j in range(n):
            acc = 0
            for k in range(n):
                acc += ai[k] * b[k][j]
            total += acc
    return total

print(matmul(120))
