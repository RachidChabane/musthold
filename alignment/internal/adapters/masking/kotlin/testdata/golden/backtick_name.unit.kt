import java.math.BigDecimal
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

class Case {
    private val helper1 = BigDecimal("4")

    private lateinit var helper2: Cart

    @BeforeEach
    fun helper3() {
        helper2 = Cart.of("ada")
    }

    @Test
    fun testCase() {
        assertThrows<IllegalArgumentException> { lineTotal(helper1, 0) }
    }
}
