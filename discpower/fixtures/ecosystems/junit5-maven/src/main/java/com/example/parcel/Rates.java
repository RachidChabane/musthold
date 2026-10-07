package com.example.parcel;

import java.math.BigDecimal;
import java.math.RoundingMode;

public final class Rates {

    public static final BigDecimal FREE_SHIPPING_FROM = new BigDecimal("50.00");
    public static final BigDecimal FLAT_FEE = new BigDecimal("4.50");
    public static final int MAX_PERCENT = 100;

    private Rates() {
    }

    public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity) {
        return unitPrice.multiply(BigDecimal.valueOf(quantity));
    }

    static int clampPercent(int percent) {
        return Math.max(0, Math.min(MAX_PERCENT, percent));
    }

    public static BigDecimal discounted(BigDecimal total, int percent) {
        int kept = MAX_PERCENT - clampPercent(percent);
        return total.multiply(BigDecimal.valueOf(kept))
                .divide(BigDecimal.valueOf(MAX_PERCENT), 2, RoundingMode.HALF_UP);
    }

    public static BigDecimal shippingFee(BigDecimal total) {
        if (total.compareTo(FREE_SHIPPING_FROM) >= 0) {
            return BigDecimal.ZERO;
        }
        return FLAT_FEE;
    }
}
