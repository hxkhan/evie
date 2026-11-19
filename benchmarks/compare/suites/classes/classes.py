class Adder:
    def __init__(self, base, step):
        self.base = base
        self.step = step

    def adjust(self, x):
        self.base = (self.base + self.step) % 1000
        return self.base + x

    def compute(self, x):
        return self.adjust(x) + self.step


class Scaler:
    def __init__(self, base, factor):
        self.base = base
        self.factor = factor

    def adjust(self, x):
        self.base = (self.base + x) % 1000
        return self.base * self.factor

    def compute(self, x):
        return self.adjust(x) - self.factor


def main():
    items = []
    for i in range(200):
        if i % 2 == 0:
            items.append(Adder(i, (i % 7) + 1))
        else:
            items.append(Scaler(i, (i % 5) + 1))

    total = 0
    for r in range(2000):
        x = r % 13
        for item in items:
            total += item.compute(x)
    return total


print(main())
