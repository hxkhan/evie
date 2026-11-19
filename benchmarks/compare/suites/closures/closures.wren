class Closures {
  static makeAcc(start) {
    var total = start
    return Fn.new {|delta|
      total = total + delta
      return total
    }
  }

  static run() {
    var sum = 0
    var i = 0
    while (i < 50000) {
      var acc = makeAcc(i)
      acc.call(3)
      acc.call(7)
      sum = sum + acc.call(-2)
      i = i + 1
    }

    var big = makeAcc(0)
    var j = 0
    while (j < 1000000) {
      big.call(j % 7)
      j = j + 1
    }
    sum = sum + big.call(0)
    return sum
  }
}

System.print(Closures.run())
