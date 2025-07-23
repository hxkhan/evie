var fib = null

fib = Fn.new {|n|
    if (n < 2) return n
    
    return fib.call(n-1) + fib.call(n-2)
}

System.print(fib.call(35))