package parcel

const val FREE_SHIPPING_FROM = 5000
const val FLAT_FEE = 450
const val MAX_PERCENT = 100

fun lineTotal(unitPrice: Int, quantity: Int): Int {
    return unitPrice * quantity
}

internal fun clampPercent(percent: Int): Int {
    return maxOf(0, minOf(MAX_PERCENT, percent))
}

fun discounted(total: Int, percent: Int): Int {
    val kept = MAX_PERCENT - clampPercent(percent)
    return total * kept / MAX_PERCENT
}

fun shippingFee(total: Int): Int {
    if (total >= FREE_SHIPPING_FROM) {
        return 0
    }
    return FLAT_FEE
}
