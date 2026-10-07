import static org.junit.jupiter.api.Assertions.assertEquals;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

class Case {
    private Cart helper1;

    @BeforeEach
    void helper2() {
        helper1 = new Cart("ada");
    }

    @ParameterizedTest
    @ValueSource(ints = {1, 2, 3})
    void testCase(int kilos) {
        assertEquals(true, Pricing.shippingCost(kilos, Zone.LOCAL).signum() > 0);
    }
}
