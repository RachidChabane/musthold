import java.util.stream.Stream
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.Arguments
import org.junit.jupiter.params.provider.MethodSource

class Case {
    private lateinit var helper1: Cart

    @BeforeEach
    fun helper2() {
        helper1 = Cart.of("ada")
    }

    companion object {
        @JvmStatic
        fun helper3(): Stream<Arguments> = Stream.of(Arguments.of(2, Zone.REMOTE), Arguments.of(2, Zone.LOCAL))
    }

    @ParameterizedTest
    @MethodSource("helper3")
    fun testCase(kilos: Int, zone: Zone) {
        assertEquals(zone == Zone.REMOTE, shippingCost(kilos, zone) > shippingCost(kilos, Zone.LOCAL))
    }
}
