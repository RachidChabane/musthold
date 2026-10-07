package shop;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static shop.Pricing.lineTotal;

import java.math.BigDecimal;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.junit.jupiter.params.provider.ValueSource;

class PricingTest {
    private static final BigDecimal UNIT = new BigDecimal("4");
    private Cart cart;

    @BeforeEach
    void openCart() {
        cart = new Cart("ada");
    }

    @Test
    @DisplayName("multiplies the unit price by the quantity [verifies PAR-1~1]")
    void totalsTheLine() {
        // four units at four each
        assertEquals(0, new BigDecimal("16").compareTo(lineTotal(UNIT, 4)));
    }

    @Test
    void rejectsAZeroQuantity() {
        assertThrows(IllegalArgumentException.class, () -> lineTotal(UNIT, 0));
    }

    @ParameterizedTest(name = "{0} kilos to {1} cost more [verifies PAR-2~1]")
    @MethodSource("parcels")
    void remoteCostsMore(int kilos, Zone zone) {
        assertEquals(zone.isRemote(), Pricing.shippingCost(kilos, zone).compareTo(Pricing.shippingCost(kilos, Zone.LOCAL)) > 0);
    }

    static Stream<Arguments> parcels() {
        return Stream.of(Arguments.of(2, Zone.REMOTE), Arguments.of(2, Zone.LOCAL));
    }

    @ParameterizedTest
    @ValueSource(ints = {1, 2, 3})
    void anyWeightHasAPrice(int kilos) {
        assertEquals(true, Pricing.shippingCost(kilos, Zone.LOCAL).signum() > 0);
    }

    @Nested
    @DisplayName("a cart with lines")
    class WithLines {
        @BeforeEach
        void addLine() {
            cart.add(UNIT, 2);
        }

        @Test
        @DisplayName("adds the lines up [verifies PAR-1~1]")
        void sums() {
            assertEquals(0, new BigDecimal("8").compareTo(cart.total()));
        }
    }
}
