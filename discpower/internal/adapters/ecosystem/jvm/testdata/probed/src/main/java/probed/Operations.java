package probed;

import java.math.BigDecimal;
import java.math.RoundingMode;
import java.time.DayOfWeek;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;

public final class Operations {

    private Operations() {
    }

    public static int divide(int dividend, int divisor) {
        return dividend / divisor;
    }

    public static DayOfWeek weekday(LocalDate day) {
        return day.getDayOfWeek();
    }

    public static LocalDateTime shift(LocalDateTime start, long hours) {
        return start.plusHours(hours);
    }

    public static Instant epoch(Instant moment) {
        return moment;
    }

    public static BigDecimal scaled(BigDecimal amount, int digits) {
        return amount.setScale(digits, RoundingMode.HALF_UP);
    }

    public static double half(double value) {
        return value / 2;
    }

    public static boolean isPositive(long value) {
        return value > 0;
    }

    public static String echo(String text) {
        return text;
    }

    public static Integer keep(Integer value) {
        return value;
    }

    public static int fail() {
        throw new IllegalStateException("refused");
    }

    public static void crash() {
        System.exit(3);
    }

    public static int spin() {
        while (true) {
            Thread.onSpinWait();
        }
    }

    static int hidden(int value) {
        return value + 1;
    }

    public static int overloaded(int value) {
        return value;
    }

    public static long overloaded(long value) {
        return value;
    }
}
