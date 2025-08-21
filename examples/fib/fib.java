class fib {
    public static void main(String[] args) {
        System.out.println(calc(35));
    }

    private static int calc(int n) {
        if (n < 2)
            return n;

        return calc(n - 1) + calc(n - 2);
    }
}