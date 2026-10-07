package com.example.parcel;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.math.BigDecimal;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

class RatesTest {

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    void multipliesUnitPriceByQuantity() {
        assertEquals(new BigDecimal("13.50"), Rates.lineTotal(new BigDecimal("4.50"), 3));
    }

    @Test
    @DisplayName("is zero for no units")
    void isZeroForNoUnits() {
        assertEquals(0, BigDecimal.ZERO.compareTo(Rates.lineTotal(new BigDecimal("7.00"), 0)));
    }

    @ParameterizedTest(name = "takes {0} percent off {1} [verifies PAR-2~1]")
    @DisplayName("percentage discount")
    @CsvSource({
            "10, 200.00, 180.00",
            "50, 80.00, 40.00",
            "0, 19.99, 19.99",
            "150, 80.00, 0.00",
            "-5, 80.00, 80.00"
    })
    void takesPercentOff(int percent, BigDecimal total, BigDecimal expected) {
        assertEquals(expected, Rates.discounted(total, percent));
    }

    @Nested
    @DisplayName("shipping [verifies PAR-3~1]")
    class Shipping {

        @Test
        @DisplayName("is waived from fifty")
        void isWaivedFromFifty() {
            assertEquals(0, BigDecimal.ZERO.compareTo(Rates.shippingFee(Rates.FREE_SHIPPING_FROM)));
        }

        @Test
        @DisplayName("is charged below fifty")
        void isChargedBelowFifty() {
            assertEquals(Rates.FLAT_FEE, Rates.shippingFee(Rates.FREE_SHIPPING_FROM.subtract(new BigDecimal("0.01"))));
        }
    }
}
