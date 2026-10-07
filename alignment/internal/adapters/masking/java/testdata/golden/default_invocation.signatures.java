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

public final class Pricing {
    public static final BigDecimal BASE_RATE;
    public static final BigDecimal REMOTE_SURCHARGE;
    public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity);
    public static BigDecimal shippingCost(int kilos, Zone zone);
    static int clamp(int percent);
}

public enum Zone {
    LOCAL, REMOTE;
    public boolean isRemote();
}
