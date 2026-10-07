class Cart(private val owner: String) : Iterable<BigDecimal> {
    var created: Int
    fun push(price: BigDecimal)
    fun add(price: BigDecimal, quantity: Int = 1)
    val total: BigDecimal
    override fun iterator(): Iterator<BigDecimal>
    companion object {
        const val EMPTY
        fun of(owner: String): Cart
    }
}

enum class Zone {
    LOCAL, REMOTE
}

fun shippingCost(kilos: Int, zone: Zone): BigDecimal
