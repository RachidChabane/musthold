package parcel

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.DisplayName
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.CsvSource

class RatesTest {

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    fun multipliesUnitPriceByQuantity() {
        assertEquals(12, lineTotal(4, 3))
    }

    @Test
    @DisplayName("is zero for no units")
    fun isZeroForNoUnits() {
        assertEquals(0, lineTotal(7, 0))
    }

    @ParameterizedTest(name = "takes {0} percent off {1} [verifies PAR-2~1]")
    @CsvSource(
        "10, 20000, 18000",
        "50, 8000, 4000",
        "0, 1999, 1999",
        "150, 8000, 0",
        "-5, 8000, 8000",
    )
    fun takesPercentOff(percent: Int, total: Int, expected: Int) {
        assertEquals(expected, discounted(total, percent))
    }

    @Test
    @DisplayName("is waived from fifty [verifies PAR-3~1]")
    fun isWaivedFromFifty() {
        assertEquals(0, shippingFee(FREE_SHIPPING_FROM))
        assertEquals(FLAT_FEE, shippingFee(FREE_SHIPPING_FROM - 1))
    }
}
