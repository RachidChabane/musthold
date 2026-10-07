import static org.junit.jupiter.api.Assertions.assertEquals;
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

    @BeforeEach
    void helper4() {
        helper2.add(helper1, 2);
    }

    @Test
    void testCase() {
        assertEquals(0, new BigDecimal("8").compareTo(helper2.total()));
    }
}
