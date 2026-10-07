package parcel;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

import java.math.BigDecimal;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

class PricingTest {
    private static final String LOCAL = "local";
    private static final String REMOTE = "remote";

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    void multipliesTheUnitPrice() {
        assertEquals(0, new BigDecimal("12").compareTo(Pricing.lineTotal(new BigDecimal("4"), 3)));
        assertEquals(0, new BigDecimal("5").compareTo(Pricing.lineTotal(new BigDecimal("5"), 1)));
    }

    @Test
    @DisplayName("costs the price of a single unit [verifies PAR-1~1]")
    void singleUnit() {
        assertEquals(0, new BigDecimal("7").compareTo(Pricing.lineTotal(new BigDecimal("7"), 1)));
    }

    @Test
    @DisplayName("refuses a negative quantity [verifies PAR-1~1]")
    void negativeQuantity() {
        assertThrows(IllegalArgumentException.class, () -> Pricing.lineTotal(BigDecimal.ONE, -1));
    }

    @Test
    @DisplayName("refuses a zero quantity [verifies PAR-3~1]")
    void zeroQuantity() {
        assertThrows(IllegalArgumentException.class, () -> Pricing.lineTotal(new BigDecimal("4"), 0));
    }

    @Test
    @DisplayName("adds the remote surcharge [verifies PAR-2~1]")
    void remoteSurcharge() {
        BigDecimal local = Pricing.shippingCost(2, LOCAL);
        BigDecimal remote = Pricing.shippingCost(2, REMOTE);
        assertEquals(0, new BigDecimal("4.50").compareTo(remote.subtract(local)));
    }

    @Test
    @DisplayName("quotes a number [verifies PAR-2~1]")
    void quotedNumber() {
        assertNotNull(Pricing.shippingCost(1, LOCAL));
    }
}
