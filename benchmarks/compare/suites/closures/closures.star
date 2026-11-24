# a Starlark closure cannot reassign a variable of the function around it, so
# the accumulator keeps its total in a one-element list
def make_acc(start):
    total = [start]

    def add(delta):
        total[0] += delta
        return total[0]

    return add

def main():
    total_sum = 0
    for i in range(50000):
        acc = make_acc(i)
        acc(3)
        acc(7)
        total_sum += acc(-2)

    big = make_acc(0)
    for j in range(1000000):
        big(j % 7)
    total_sum += big(0)
    return total_sum

print(main())
