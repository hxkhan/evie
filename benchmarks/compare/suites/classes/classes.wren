class Adder {
  construct new(base, step) {
    _base = base
    _step = step
  }

  adjust(x) {
    _base = (_base + _step) % 1000
    return _base + x
  }

  compute(x) {
    return adjust(x) + _step
  }
}

class Scaler {
  construct new(base, factor) {
    _base = base
    _factor = factor
  }

  adjust(x) {
    _base = (_base + x) % 1000
    return _base * _factor
  }

  compute(x) {
    return adjust(x) - _factor
  }
}

class Runner {
  static run() {
    var items = []
    var i = 0
    while (i < 200) {
      if (i % 2 == 0) {
        items.add(Adder.new(i, (i % 7) + 1))
      } else {
        items.add(Scaler.new(i, (i % 5) + 1))
      }
      i = i + 1
    }

    var total = 0
    var r = 0
    while (r < 2000) {
      var x = r % 13
      for (item in items) {
        total = total + item.compute(x)
      }
      r = r + 1
    }
    return total
  }
}

System.print(Runner.run())
