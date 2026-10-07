public class Cart implements Iterable<BigDecimal> {
    public static int created;
    public Cart(String owner);
    public void add(BigDecimal price, int quantity);
    public BigDecimal total();
    public static class Receipt {
        public final BigDecimal amount;
        public Receipt(BigDecimal amount);
    }
}
