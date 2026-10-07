package ledger;

import java.math.BigDecimal;
import java.util.List;

public final class Totals {
    public static final BigDecimal TAX_RATE = new BigDecimal("0.20");

    private Totals() {
    }

    public static BigDecimal lineAmount(BigDecimal price, int quantity) {
        return price.multiply(BigDecimal.valueOf(quantity));
    }

    public static BigDecimal invoiceTotal(List<Line> lines) {
        BigDecimal total = BigDecimal.ZERO;
        for (Line line : lines) {
            total = total.add(lineAmount(line.price(), line.quantity()));
        }
        return total;
    }

    public static String formatTotal(BigDecimal total) {
        return total.setScale(2) + " EUR";
    }
}
