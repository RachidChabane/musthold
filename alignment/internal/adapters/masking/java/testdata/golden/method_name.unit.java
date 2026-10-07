import static org.junit.jupiter.api.Assertions.assertThrows;
import static shop.Pricing.lineTotal;
import java.math.BigDecimal;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class Case {
    private static final BigDecimal helper1 = new BigDecimal("4");

    private Cart helper2;

    @BeforeEach
    void helper3() {
        helper2 = new Cart("ada");
    }

    @Test
    void testCase() {
        assertThrows(IllegalArgumentException.class, () -> lineTotal(helper1, 0));
    }
}
