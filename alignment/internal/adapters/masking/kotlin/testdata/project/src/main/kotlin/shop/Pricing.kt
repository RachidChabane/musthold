package shop

import java.math.BigDecimal

val BASE_RATE: BigDecimal = BigDecimal("5.00")
const val LIMIT = 100

enum class Zone { LOCAL, REMOTE }

/** Totals one line of an order. */
fun lineTotal(unitPrice: BigDecimal, quantity: Int): BigDecimal {
    require(quantity > 0) { "quantity must be positive" }
    return unitPrice.multiply(BigDecimal.valueOf(quantity.toLong()))
}

fun shippingCost(kilos: Int, zone: Zone): BigDecimal =
    if (zone == Zone.REMOTE) BASE_RATE.add(BigDecimal.valueOf(kilos.toLong())) else BASE_RATE

private fun audit() {}

object Rates {
    const val SURCHARGE = 4
    fun base(): Int = 1
    private fun secret(): Int = 2
}
