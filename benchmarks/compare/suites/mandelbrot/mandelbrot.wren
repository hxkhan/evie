class Mandelbrot {
  static run(size, maxIter) {
    var total = 0
    var py = 0
    while (py < size) {
      var y0 = (py / size) * 2.0 - 1.0
      var px = 0
      while (px < size) {
        var x0 = (px / size) * 3.5 - 2.5
        var x = 0.0
        var y = 0.0
        var iter = 0
        while (iter < maxIter) {
          var xx = x * x
          var yy = y * y
          if (xx + yy > 4.0) break
          y = 2.0 * x * y + y0
          x = xx - yy + x0
          iter = iter + 1
        }
        total = total + iter
        px = px + 1
      }
      py = py + 1
    }
    return total
  }
}

System.print(Mandelbrot.run(400, 50))
