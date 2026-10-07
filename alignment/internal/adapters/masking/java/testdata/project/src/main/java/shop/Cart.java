package shop;

import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.List;

public class Cart implements Iterable<BigDecimal> {
    private final List<BigDecimal> lines = new ArrayList<>();
    private final String owner;
    public static int created = 0;

    @Deprecated
    public Cart(String owner) {
        this.owner = owner;
        created++;
    }

    public void add(BigDecimal price, int quantity) {
        lines.add(Pricing.lineTotal(price, quantity));
    }

    public BigDecimal total() {
        return lines.stream().reduce(BigDecimal.ZERO, BigDecimal::add);
    }

    private void audit() {
    }

    public static class Receipt {
        public final BigDecimal amount;

        public Receipt(BigDecimal amount) {
            this.amount = amount;
        }
    }
}
