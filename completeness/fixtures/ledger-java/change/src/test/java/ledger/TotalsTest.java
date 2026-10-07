package ledger;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.math.BigDecimal;
import java.util.List;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

class TotalsTest {
    @Test
    @DisplayName("multiplies the price by the quantity [verifies INV-1~1]")
    void multipliesThePrice() {
        assertEquals(0, new BigDecimal("12").compareTo(Totals.lineAmount(new BigDecimal("4"), 3)));
    }

    @Test
    void welcomeCodeDiscountsTheTotal() {
        assertEquals(0, new BigDecimal("9.00").compareTo(Totals.applyDiscount(BigDecimal.TEN, "WELCOME")));
    }

    @Nested
    @DisplayName("an invoice")
    class Invoice {
        @Test
        @DisplayName("adds the lines [verifies INV-1~1]")
        void addsTheLines() {
            List<Line> lines = List.of(new Line(new BigDecimal("4"), 3), new Line(new BigDecimal("5"), 1));
            assertEquals(0, new BigDecimal("21.25").compareTo(Totals.invoiceTotal(lines)));
        }
    }
}
