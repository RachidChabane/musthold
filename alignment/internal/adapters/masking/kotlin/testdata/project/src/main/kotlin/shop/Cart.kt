package shop

import java.math.BigDecimal

data class Receipt(val amount: BigDecimal, val owner: String)

class Cart(private val owner: String) : Iterable<BigDecimal> {
    private val lines = mutableListOf<BigDecimal>()
    var created: Int = 0

    @Deprecated("use add")
    fun push(price: BigDecimal) {
        lines.add(price)
    }

    fun add(price: BigDecimal, quantity: Int = 1) {
        lines.add(lineTotal(price, quantity))
    }

    val total: BigDecimal
        get() = lines.fold(BigDecimal.ZERO) { sum, line -> sum + line }

    override fun iterator(): Iterator<BigDecimal> = lines.iterator()

    private fun audit() {}

    companion object {
        const val EMPTY = 0
        fun of(owner: String): Cart = Cart(owner)
    }
}
