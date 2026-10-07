package parcel;

public final class Rates {

    public static final int FREE_SHIPPING_FROM = 5000;
    public static final int FLAT_FEE = 450;
    public static final int MAX_PERCENT = 100;

    private Rates() {
    }

    public static int lineTotal(int unitPrice, int quantity) {
        return unitPrice * quantity;
    }

    static int clampPercent(int percent) {
        return Math.max(0, Math.min(MAX_PERCENT, percent));
    }

    public static int discounted(int total, int percent) {
        int kept = MAX_PERCENT - clampPercent(percent);
        return total * kept / MAX_PERCENT;
    }

    public static int shippingFee(int total) {
        if (total >= FREE_SHIPPING_FROM) {
            return 0;
        }
        return FLAT_FEE;
    }
}
