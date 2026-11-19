def main():
    keys = ["k" + str(i) for i in range(500)]

    total = 0
    for r in range(400):
        m = {}
        for j in range(500):
            m[keys[j]] = j + r
        for j in range(500):
            k = keys[j]
            m[k] = m[k] + (j % 5)
        for j in range(500):
            total += m[keys[j]]
    return total

print(main())
