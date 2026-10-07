package parcel;

import java.math.BigDecimal;

public final class Pricing {
    public static final BigDecimal BASE_RATE = new BigDecimal("5.00");
    public static final BigDecimal PER_KILO = new BigDecimal("2.50");
    public static final BigDecimal REMOTE_SURCHARGE = new BigDecimal("4.50");

    private Pricing() {
    }

    public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) {
        if (quantity <= 0) {
            throw new IllegalArgumentException("quantity must be positive");
        }
        return unitPrice.multiply(BigDecimal.valueOf(quantity));
    }

    public static BigDecimal shippingCost(int weight, String zone) {
        BigDecimal cost = BASE_RATE.add(PER_KILO.multiply(BigDecimal.valueOf(weight)));
        if ("remote".equals(zone)) {
            cost = cost.add(REMOTE_SURCHARGE);
        }
        return cost;
    }
}
