class Strings {
  static run() {
    var base = "alpha,beta,gamma,delta,epsilon"
    var total = 0
    var i = 0
    while (i < 40000) {
      var s = base + "," + i.toString
      var parts = s.split(",")
      var joined = parts.join("-")
      var replaced = joined.replace("beta", "BETA")
      if (replaced.contains("gamma")) {
        total = total + replaced.count
      }
      i = i + 1
    }
    return total
  }
}

System.print(Strings.run())
