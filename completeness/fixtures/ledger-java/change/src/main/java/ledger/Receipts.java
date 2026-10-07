package ledger;

public final class Receipts {
    static String receiptLine(String label, String amount) {
        return String.format("%-20s%10s", label, amount);
    }
}
