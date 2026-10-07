import static org.junit.jupiter.api.Assertions.assertEquals;
import java.util.stream.Stream;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

class Case {
    private Cart helper1;

    @BeforeEach
    void helper2() {
        helper1 = new Cart("ada");
    }

    static Stream<Arguments> helper3() {
        return Stream.of(Arguments.of(2, Zone.REMOTE), Arguments.of(2, Zone.LOCAL));
    }

    @ParameterizedTest
    @MethodSource("helper3")
    void testCase(int kilos, Zone zone) {
        assertEquals(zone.isRemote(), Pricing.shippingCost(kilos, zone).compareTo(Pricing.shippingCost(kilos, Zone.LOCAL)) > 0);
    }
}
