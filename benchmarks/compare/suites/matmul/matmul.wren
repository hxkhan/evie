class MatMul {
  static run(n) {
    var a = []
    var b = []
    var i = 0
    while (i < n) {
      var ra = []
      var rb = []
      var j = 0
      while (j < n) {
        ra.add((i * 3 + j * 7) % 17)
        rb.add((i * 5 + j * 11) % 19)
        j = j + 1
      }
      a.add(ra)
      b.add(rb)
      i = i + 1
    }

    var total = 0
    i = 0
    while (i < n) {
      var ai = a[i]
      var j = 0
      while (j < n) {
        var acc = 0
        var k = 0
        while (k < n) {
          acc = acc + ai[k] * b[k][j]
          k = k + 1
        }
        total = total + acc
        j = j + 1
      }
      i = i + 1
    }
    return total
  }
}

System.print(MatMul.run(120))
