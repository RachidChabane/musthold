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

    @BeforeEach
    fun helper4() {
        helper2.add(helper1, 2)
    }

    @Test
    fun testCase() {
        assertEquals(0, BigDecimal("8").compareTo(helper2.total))
    }
}
