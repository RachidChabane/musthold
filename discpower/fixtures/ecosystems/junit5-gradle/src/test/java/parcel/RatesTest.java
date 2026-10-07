package parcel;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

class RatesTest {

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    void multipliesUnitPriceByQuantity() {
        assertEquals(12, Rates.lineTotal(4, 3));
    }

    @Test
    @DisplayName("is zero for no units")
    void isZeroForNoUnits() {
        assertEquals(0, Rates.lineTotal(7, 0));
    }

    @ParameterizedTest(name = "takes {0} percent off {1} [verifies PAR-2~1]")
    @CsvSource({
            "10, 20000, 18000",
            "50, 8000, 4000",
            "0, 1999, 1999",
            "150, 8000, 0",
            "-5, 8000, 8000"
    })
    void takesPercentOff(int percent, int total, int expected) {
        assertEquals(expected, Rates.discounted(total, percent));
    }

    @Test
    @DisplayName("is waived from fifty [verifies PAR-3~1]")
    void isWaivedFromFifty() {
        assertEquals(0, Rates.shippingFee(Rates.FREE_SHIPPING_FROM));
        assertEquals(Rates.FLAT_FEE, Rates.shippingFee(Rates.FREE_SHIPPING_FROM - 1));
    }
}
