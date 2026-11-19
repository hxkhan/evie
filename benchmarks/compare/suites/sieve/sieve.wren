class Sieve {
  static run(n) {
    var flags = []
    var i = 0
    while (i < n) {
      flags.add(0)
      i = i + 1
    }

    i = 2
    while (i * i < n) {
      if (flags[i] == 0) {
        var j = i * i
        while (j < n) {
          flags[j] = 1
          j = j + i
        }
      }
      i = i + 1
    }

    var count = 0
    i = 2
    while (i < n) {
      if (flags[i] == 0) count = count + 1
      i = i + 1
    }
    return count
  }
}

System.print(Sieve.run(1000000))
