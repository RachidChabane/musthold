package ledger;

import java.math.BigDecimal;

public record Line(BigDecimal price, int quantity) {
}
