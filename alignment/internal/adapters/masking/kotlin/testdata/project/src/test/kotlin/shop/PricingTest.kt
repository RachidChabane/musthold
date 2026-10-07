package shop

import java.math.BigDecimal
import java.util.stream.Stream
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.DisplayName
import org.junit.jupiter.api.Nested
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.Arguments
import org.junit.jupiter.params.provider.MethodSource

@DisplayName("pricing")
@Tag("fast")
class PricingTest {
    private val unit = BigDecimal("4")
    private lateinit var cart: Cart

    @BeforeEach
    fun openCart() {
        cart = Cart.of("ada")
    }

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    fun totalsTheLine() {
        // four units at four each
        assertEquals(0, BigDecimal("16").compareTo(lineTotal(unit, 4)))
    }

    @Test
    fun `rejects a zero quantity`() {
        assertThrows<IllegalArgumentException> { lineTotal(unit, 0) }
    }

    @ParameterizedTest(name = "{0} kilos to {1} cost more [verifies PAR-2~1]")
    @MethodSource("parcels")
    fun remoteCostsMore(kilos: Int, zone: Zone) {
        assertEquals(zone == Zone.REMOTE, shippingCost(kilos, zone) > shippingCost(kilos, Zone.LOCAL))
    }

    companion object {
        @JvmStatic
        fun parcels(): Stream<Arguments> = Stream.of(Arguments.of(2, Zone.REMOTE), Arguments.of(2, Zone.LOCAL))
    }

    @Nested
    @DisplayName("a cart with lines")
    inner class WithLines {
        @BeforeEach
        fun addLine() {
            cart.add(unit, 2)
        }

        @Test
        @DisplayName("adds the lines up [verifies PAR-1~1]")
        fun sums() {
            assertEquals(0, BigDecimal("8").compareTo(cart.total))
        }
    }
}
