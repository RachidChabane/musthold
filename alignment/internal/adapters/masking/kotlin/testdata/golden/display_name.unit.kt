import java.math.BigDecimal
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.assertEquals

class Case {
    private val helper1 = BigDecimal("4")

    private lateinit var helper2: Cart

    @BeforeEach
    fun helper3() {
        helper2 = Cart.of("ada")
    }

    @Test
    fun testCase() {
        assertEquals(0, BigDecimal("16").compareTo(lineTotal(helper1, 4)))
    }
}
