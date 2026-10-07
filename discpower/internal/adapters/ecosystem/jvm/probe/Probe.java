import java.lang.reflect.Constructor;
import java.lang.reflect.Field;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.LocalTime;
import java.time.OffsetDateTime;
import java.time.ZoneOffset;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Date;
import java.util.List;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/**
 * Calls operations of a JVM project with typed literals and prints what each call returns or throws,
 * one JSON object per line. The request file holds one tab separated call per line:
 * CALL, class name, method name, argument count, then a type and a value for each argument.
 */
public final class Probe {

    private static final int EXIT_FAILURE = 70;
    private static final long CALL_DEADLINE_SECONDS = 5;
    private static final String TIMEOUT = "Timeout";
    private static final String ARGUMENT_ERROR = "ArgumentError";
    private static final String UNKNOWN_TARGET = "UnknownTarget";
    private static final String INSTANCE_FIELD = "INSTANCE";

    private static final class ArgumentException extends Exception {
        ArgumentException(String message) {
            super(message);
        }
    }

    private static final class Observation {
        final String kind;
        final String type;
        final String value;

        Observation(String kind, String type, String value) {
            this.kind = kind;
            this.type = type;
            this.value = value;
        }

        static Observation returns(String type, String value) {
            return new Observation("returns", type, value);
        }

        static Observation raises(String name) {
            return new Observation("raises", null, name);
        }

        String json() {
            StringBuilder out = new StringBuilder("{\"kind\":").append(quote(kind));
            if (type != null) {
                out.append(",\"type\":").append(quote(type));
            }
            return out.append(",\"value\":").append(quote(value)).append('}').toString();
        }
    }

    private record Literal(String type, String value) {
    }

    private record Call(String className, String methodName, List<Literal> arguments) {
    }

    public static void main(String[] args) throws Exception {
        if (args.length != 2) {
            System.err.println("usage: Probe REQUEST RESPONSE");
            System.exit(EXIT_FAILURE);
        }
        List<Call> calls;
        try {
            calls = readCalls(Path.of(args[0]));
        } catch (Exception e) {
            System.err.println("unreadable request: " + e);
            System.exit(EXIT_FAILURE);
            return;
        }
        ExecutorService executor = Executors.newSingleThreadExecutor(runnable -> {
            Thread thread = new Thread(runnable, "probe");
            thread.setDaemon(true);
            return thread;
        });
        try (var sink = Files.newBufferedWriter(Path.of(args[1]), StandardCharsets.UTF_8)) {
            for (Call call : calls) {
                Observation observation = observe(executor, call);
                sink.write(observation.json());
                sink.write('\n');
                sink.flush();
                if (TIMEOUT.equals(observation.value)) {
                    Runtime.getRuntime().halt(0);
                }
            }
        }
        Runtime.getRuntime().halt(0);
    }

    private static List<Call> readCalls(Path file) throws Exception {
        List<Call> calls = new ArrayList<>();
        for (String line : Files.readAllLines(file, StandardCharsets.UTF_8)) {
            if (line.isEmpty()) {
                continue;
            }
            String[] fields = line.split("\t", -1);
            int count = Integer.parseInt(fields[3]);
            List<Literal> arguments = new ArrayList<>();
            for (int index = 0; index < count; index++) {
                arguments.add(new Literal(fields[4 + 2 * index], unescape(fields[5 + 2 * index])));
            }
            calls.add(new Call(fields[1], fields[2], arguments));
        }
        return calls;
    }

    private static String unescape(String text) {
        StringBuilder out = new StringBuilder();
        for (int index = 0; index < text.length(); index++) {
            char current = text.charAt(index);
            if (current != '\\' || index + 1 == text.length()) {
                out.append(current);
                continue;
            }
            char next = text.charAt(++index);
            switch (next) {
                case 't' -> out.append('\t');
                case 'n' -> out.append('\n');
                case 'r' -> out.append('\r');
                default -> out.append(next);
            }
        }
        return out.toString();
    }

    private static Observation observe(ExecutorService executor, Call call) {
        Future<Observation> pending = executor.submit(() -> run(call));
        try {
            return pending.get(CALL_DEADLINE_SECONDS, TimeUnit.SECONDS);
        } catch (TimeoutException e) {
            return Observation.raises(TIMEOUT);
        } catch (InterruptedException | ExecutionException e) {
            return Observation.raises(nameOf(e));
        }
    }

    private static Observation run(Call call) {
        List<Method> candidates;
        try {
            candidates = candidates(Class.forName(call.className()), call);
        } catch (ClassNotFoundException | LinkageError e) {
            return Observation.raises(UNKNOWN_TARGET);
        }
        if (candidates.isEmpty()) {
            return Observation.raises(UNKNOWN_TARGET);
        }
        for (Method method : candidates) {
            Object[] converted;
            try {
                converted = convert(method.getParameterTypes(), call.arguments());
            } catch (ArgumentException e) {
                continue;
            }
            return invoke(method, converted);
        }
        return Observation.raises(ARGUMENT_ERROR);
    }

    private static List<Method> candidates(Class<?> type, Call call) {
        List<Method> found = new ArrayList<>();
        for (Method method : type.getDeclaredMethods()) {
            if (method.getName().equals(call.methodName())
                    && method.getParameterCount() == call.arguments().size()
                    && !method.isSynthetic()
                    && !method.isBridge()) {
                found.add(method);
            }
        }
        return found;
    }

    private static Observation invoke(Method method, Object[] arguments) {
        try {
            method.setAccessible(true);
            Object receiver = Modifier.isStatic(method.getModifiers()) ? null : receiverOf(method.getDeclaringClass());
            return describe(method.invoke(receiver, arguments));
        } catch (InvocationTargetException e) {
            return Observation.raises(nameOf(e.getCause() == null ? e : e.getCause()));
        } catch (ReflectiveOperationException | RuntimeException | LinkageError e) {
            return Observation.raises(nameOf(e));
        }
    }

    private static Object receiverOf(Class<?> type) throws ReflectiveOperationException {
        try {
            Field singleton = type.getDeclaredField(INSTANCE_FIELD);
            if (Modifier.isStatic(singleton.getModifiers())) {
                singleton.setAccessible(true);
                return singleton.get(null);
            }
        } catch (NoSuchFieldException e) {
            // A class without a singleton field is built with its default constructor.
        }
        Constructor<?> constructor = type.getDeclaredConstructor();
        constructor.setAccessible(true);
        return constructor.newInstance();
    }

    private static String nameOf(Throwable error) {
        Throwable cause = error;
        while ((cause instanceof ExecutionException || cause instanceof InvocationTargetException) && cause.getCause() != null) {
            cause = cause.getCause();
        }
        String simple = cause.getClass().getSimpleName();
        return simple.isEmpty() ? cause.getClass().getName() : simple;
    }

    private static Object[] convert(Class<?>[] types, List<Literal> literals) throws ArgumentException {
        Object[] converted = new Object[types.length];
        for (int index = 0; index < types.length; index++) {
            converted[index] = convert(types[index], literals.get(index));
        }
        return converted;
    }

    private static Object convert(Class<?> type, Literal literal) throws ArgumentException {
        try {
            return convertValue(type, literal);
        } catch (NumberFormatException | ArithmeticException | DateTimeParseException e) {
            throw new ArgumentException(e.getMessage());
        }
    }

    private static Object convertValue(Class<?> type, Literal literal) throws ArgumentException {
        switch (literal.type()) {
            case "missing":
                throw new ArgumentException("missing argument");
            case "null":
                if (type.isPrimitive()) {
                    throw new ArgumentException("null for a primitive");
                }
                return null;
            case "integer":
            case "decimal":
                return number(type, literal.value());
            case "boolean":
                return bool(type, literal.value());
            case "string":
                return text(type, literal.value());
            case "datetime":
                return moment(type, literal.value());
            default:
                throw new ArgumentException("unknown literal type " + literal.type());
        }
    }

    private static Object number(Class<?> type, String value) throws ArgumentException {
        BigDecimal decimal = new BigDecimal(value);
        if (type == int.class || type == Integer.class) {
            return decimal.intValueExact();
        }
        if (type == long.class || type == Long.class) {
            return decimal.longValueExact();
        }
        if (type == short.class || type == Short.class) {
            return decimal.shortValueExact();
        }
        if (type == byte.class || type == Byte.class) {
            return decimal.byteValueExact();
        }
        if (type == double.class || type == Double.class) {
            return decimal.doubleValue();
        }
        if (type == float.class || type == Float.class) {
            return decimal.floatValue();
        }
        if (type == BigDecimal.class || type == Number.class) {
            return decimal;
        }
        if (type == BigInteger.class) {
            return decimal.toBigIntegerExact();
        }
        if (type == String.class || type == CharSequence.class) {
            return value;
        }
        if (type == Object.class) {
            return decimal.scale() <= 0 ? integral(decimal) : decimal;
        }
        throw new ArgumentException("a number for " + type.getName());
    }

    private static Object integral(BigDecimal decimal) {
        BigInteger whole = decimal.toBigIntegerExact();
        return whole.bitLength() < Long.SIZE ? (Object) whole.longValueExact() : whole;
    }

    private static Object bool(Class<?> type, String value) throws ArgumentException {
        if (type == boolean.class || type == Boolean.class || type == Object.class) {
            return Boolean.parseBoolean(value);
        }
        if (type == String.class || type == CharSequence.class) {
            return value;
        }
        throw new ArgumentException("a boolean for " + type.getName());
    }

    private static Object text(Class<?> type, String value) throws ArgumentException {
        if (type == String.class || type == CharSequence.class || type == Object.class) {
            return value;
        }
        if ((type == char.class || type == Character.class) && value.length() == 1) {
            return value.charAt(0);
        }
        if (type.isEnum()) {
            return enumConstant(type, value);
        }
        if (type == BigDecimal.class) {
            return new BigDecimal(value);
        }
        throw new ArgumentException("a string for " + type.getName());
    }

    @SuppressWarnings({"unchecked", "rawtypes"})
    private static Object enumConstant(Class<?> type, String value) throws ArgumentException {
        try {
            return Enum.valueOf((Class<? extends Enum>) type, value);
        } catch (IllegalArgumentException e) {
            throw new ArgumentException("no constant " + value);
        }
    }

    private static Object moment(Class<?> type, String value) throws ArgumentException {
        if (type == LocalDate.class) {
            return value.contains("T") ? LocalDateTime.parse(value).toLocalDate() : LocalDate.parse(value);
        }
        if (type == LocalDateTime.class) {
            return value.contains("T") ? LocalDateTime.parse(value) : LocalDate.parse(value).atStartOfDay();
        }
        if (type == LocalTime.class) {
            return LocalTime.parse(value);
        }
        if (type == Instant.class) {
            return instant(value);
        }
        if (type == OffsetDateTime.class) {
            return zoned(value).toOffsetDateTime();
        }
        if (type == ZonedDateTime.class) {
            return zoned(value);
        }
        if (type == Date.class) {
            return Date.from(instant(value));
        }
        if (type == String.class || type == Object.class) {
            return value;
        }
        throw new ArgumentException("a datetime for " + type.getName());
    }

    private static Instant instant(String value) {
        return zoned(value).toInstant();
    }

    private static ZonedDateTime zoned(String value) {
        if (!value.contains("T")) {
            return LocalDate.parse(value).atStartOfDay(ZoneOffset.UTC);
        }
        try {
            return ZonedDateTime.parse(value);
        } catch (DateTimeParseException e) {
            return LocalDateTime.parse(value).atZone(ZoneOffset.UTC);
        }
    }

    private static Observation describe(Object value) {
        if (value == null) {
            return Observation.returns("null", "null");
        }
        if (value instanceof Boolean flag) {
            return Observation.returns("boolean", flag.toString());
        }
        if (value instanceof Integer || value instanceof Long || value instanceof Short || value instanceof Byte
                || value instanceof BigInteger) {
            return Observation.returns("integer", value.toString());
        }
        if (value instanceof BigDecimal decimal) {
            return Observation.returns("decimal", decimal.toPlainString());
        }
        if (value instanceof Double || value instanceof Float) {
            return real(value);
        }
        if (value instanceof CharSequence || value instanceof Character) {
            return Observation.returns("string", value.toString());
        }
        return moment(value).orElseGet(() -> Observation.returns("string", render(value)));
    }

    private static Observation real(Object value) {
        double number = ((Number) value).doubleValue();
        if (Double.isNaN(number) || Double.isInfinite(number)) {
            return Observation.returns("string", Double.toString(number));
        }
        return Observation.returns("decimal", new BigDecimal(value.toString()).toPlainString());
    }

    private static java.util.Optional<Observation> moment(Object value) {
        String text = null;
        if (value instanceof LocalDate date) {
            text = DateTimeFormatter.ISO_LOCAL_DATE.format(date);
        } else if (value instanceof LocalDateTime dateTime) {
            text = DateTimeFormatter.ISO_LOCAL_DATE_TIME.format(dateTime);
        } else if (value instanceof LocalTime time) {
            text = DateTimeFormatter.ISO_LOCAL_TIME.format(time);
        } else if (value instanceof OffsetDateTime offset) {
            text = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(offset);
        } else if (value instanceof ZonedDateTime zoned) {
            text = DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(zoned);
        } else if (value instanceof Instant instant) {
            text = DateTimeFormatter.ISO_INSTANT.format(instant);
        } else if (value instanceof Date date) {
            text = DateTimeFormatter.ISO_INSTANT.format(date.toInstant());
        }
        return java.util.Optional.ofNullable(text).map(found -> Observation.returns("datetime", found));
    }

    private static String render(Object value) {
        if (value instanceof Object[] array) {
            return Arrays.deepToString(array);
        }
        return String.valueOf(value);
    }

    private static String quote(String text) {
        StringBuilder out = new StringBuilder("\"");
        for (int index = 0; index < text.length(); index++) {
            char current = text.charAt(index);
            switch (current) {
                case '"' -> out.append("\\\"");
                case '\\' -> out.append("\\\\");
                case '\n' -> out.append("\\n");
                case '\r' -> out.append("\\r");
                case '\t' -> out.append("\\t");
                default -> {
                    if (current < 0x20) {
                        out.append(String.format("\\u%04x", (int) current));
                    } else {
                        out.append(current);
                    }
                }
            }
        }
        return out.append('"').toString();
    }
}
