class Objects {
  static run() {
    var keys = []
    var i = 0
    while (i < 500) {
      keys.add("k%(i)")
      i = i + 1
    }

    var total = 0
    var r = 0
    while (r < 400) {
      var m = {}
      var j = 0
      while (j < 500) {
        m[keys[j]] = j + r
        j = j + 1
      }
      j = 0
      while (j < 500) {
        var k = keys[j]
        m[k] = m[k] + (j % 5)
        j = j + 1
      }
      j = 0
      while (j < 500) {
        total = total + m[keys[j]]
        j = j + 1
      }
      r = r + 1
    }
    return total
  }
}

System.print(Objects.run())
