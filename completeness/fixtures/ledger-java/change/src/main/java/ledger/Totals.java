package ledger;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.util.List;

public final class Totals {
    public static final BigDecimal TAX_RATE = new BigDecimal("0.25");

    private Totals() {
    }

    public static BigDecimal lineAmount(BigDecimal price, int quantity) {
        if (quantity < 0) {
            throw new IllegalArgumentException("quantity must not be negative");
        }
        return price.multiply(BigDecimal.valueOf(quantity));
    }

    public static BigDecimal invoiceTotal(List<Line> lines) {
        BigDecimal net = BigDecimal.ZERO;
        for (Line line : lines) {
            net = net.add(lineAmount(line.price(), line.quantity()));
        }
        return net.multiply(BigDecimal.ONE.add(TAX_RATE)).setScale(2, RoundingMode.HALF_UP);
    }

    public static BigDecimal applyDiscount(BigDecimal total, String code) {
        if ("WELCOME".equals(code)) {
            return total.multiply(new BigDecimal("0.90"));
        }
        return total;
    }

    /**
     * Renders a total for display.
     */
    public static String formatTotal(BigDecimal total) {
        return total.setScale(2) + " EUR";
    }
}
