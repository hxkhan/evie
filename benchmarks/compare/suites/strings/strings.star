def main():
    base = "alpha,beta,gamma,delta,epsilon"
    total = 0
    for i in range(40000):
        s = base + "," + str(i)
        parts = s.split(",")
        joined = "-".join(parts)
        replaced = joined.replace("beta", "BETA")
        if "gamma" in replaced:
            total += len(replaced)
    return total

print(main())
