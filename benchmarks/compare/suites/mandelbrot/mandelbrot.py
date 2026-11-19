def mandelbrot(size, max_iter):
    total = 0
    for py in range(size):
        y0 = (py / size) * 2.0 - 1.0
        for px in range(size):
            x0 = (px / size) * 3.5 - 2.5
            x = 0.0
            y = 0.0
            it = 0
            while it < max_iter:
                xx = x * x
                yy = y * y
                if xx + yy > 4.0:
                    break
                y = 2.0 * x * y + y0
                x = xx - yy + x0
                it += 1
            total += it
    return total

print(mandelbrot(400, 50))
