package shop;

import java.math.BigDecimal;

public final class Pricing {
    public static final BigDecimal BASE_RATE = new BigDecimal("5.00");
    public static final BigDecimal REMOTE_SURCHARGE = new BigDecimal("4.50");
    private static final int LIMIT = 100;

    private Pricing() {
    }

    /** Totals one line of an order. */
    public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) {
        if (quantity <= 0) {
            throw new IllegalArgumentException("quantity must be positive");
        }
        return unitPrice.multiply(BigDecimal.valueOf(quantity));
    }

    public static BigDecimal shippingCost(int kilos, Zone zone) {
        BigDecimal cost = BASE_RATE.add(BigDecimal.valueOf(kilos));
        return zone == Zone.REMOTE ? cost.add(REMOTE_SURCHARGE) : cost;
    }

    static int clamp(int percent) {
        return Math.min(LIMIT, percent);
    }
}
