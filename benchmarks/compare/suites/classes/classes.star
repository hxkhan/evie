# Starlark has no classes: an object is a dict of state and closures, as in the Tengo port

def new_adder(base, step):
    self = {"base": base, "step": step}

    def adjust(x):
        self["base"] = (self["base"] + self["step"]) % 1000
        return self["base"] + x

    def compute(x):
        return self["adjust"](x) + self["step"]

    self["adjust"] = adjust
    self["compute"] = compute
    return self

def new_scaler(base, factor):
    self = {"base": base, "factor": factor}

    def adjust(x):
        self["base"] = (self["base"] + x) % 1000
        return self["base"] * self["factor"]

    def compute(x):
        return self["adjust"](x) - self["factor"]

    self["adjust"] = adjust
    self["compute"] = compute
    return self

def main():
    items = []
    for i in range(200):
        if i % 2 == 0:
            items.append(new_adder(i, (i % 7) + 1))
        else:
            items.append(new_scaler(i, (i % 5) + 1))

    total = 0
    for r in range(2000):
        x = r % 13
        for item in items:
            total += item["compute"](x)
    return total

print(main())
