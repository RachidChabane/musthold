package format_test

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
)

const maxJSONDepth = 3

// jsonValues generates JSON values: nested objects and arrays of strings,
// integers, finite numbers, booleans and null.
func jsonValues(depth int) *rapid.Generator[any] {
	scalars := rapid.OneOf(
		rapid.Map(rapid.String(), func(s string) any { return s }),
		rapid.Map(rapid.Int64Range(-1_000_000, 1_000_000), func(n int64) any { return n }),
		rapid.Map(rapid.Float64Range(-1e6, 1e6), func(f float64) any { return f }),
		rapid.Map(rapid.Bool(), func(b bool) any { return b }),
		rapid.Just[any](nil),
	)
	if depth == 0 {
		return scalars
	}
	nested := rapid.Deferred(func() *rapid.Generator[any] { return jsonValues(depth - 1) })
	return rapid.OneOf(
		scalars,
		rapid.Map(rapid.SliceOfN(nested, 0, 4), func(items []any) any { return items }),
		rapid.Map(rapid.MapOfN(rapid.String(), nested, 0, 4), func(members map[string]any) any { return members }),
	)
}

// writeWithKeyOrder serializes value as JSON with object keys in the order that
// order returns, which a Go encoder never does by itself.
func writeWithKeyOrder(buffer *bytes.Buffer, value any, order func([]string) []string) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		buffer.WriteByte('{')
		for index, key := range order(keys) {
			if index > 0 {
				buffer.WriteByte(',')
			}
			encodedKey, _ := json.Marshal(key)
			buffer.Write(encodedKey)
			buffer.WriteByte(':')
			writeWithKeyOrder(buffer, typed[key], order)
		}
		buffer.WriteByte('}')
	case []any:
		buffer.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				buffer.WriteByte(',')
			}
			writeWithKeyOrder(buffer, item, order)
		}
		buffer.WriteByte(']')
	default:
		encoded, _ := json.Marshal(typed)
		buffer.Write(encoded)
	}
}

func TestCanonicalization(t *testing.T) {
	vectors := []struct {
		name  string
		input string
		want  string
	}{
		{"keys are sorted", `{"b":2,"a":1}`, `{"a":1,"b":2}`},
		{"insignificant whitespace is dropped", "{ \"a\" : [ 1 , 2 ] }\n", `{"a":[1,2]}`},
		{"nested keys are sorted", `{"z":{"y":1,"x":2},"a":null}`, `{"a":null,"z":{"x":2,"y":1}}`},
		{"numbers use the shortest ECMAScript form", `{"a":1.0,"b":1e2,"c":333333333.33333329,"d":0.000001}`, `{"a":1,"b":100,"c":333333333.3333333,"d":0.000001}`},
		{"keys sort by UTF-16 code units", "{\"\\u20ac\":1,\"\\r\":2,\"\\ufb33\":3,\"1\":4,\"\\ud83d\\ude00\":5}", "{\"\\r\":2,\"1\":4,\"€\":1,\"😀\":5,\"\ufb33\":3}"},
		{"strings keep unicode unescaped and escape controls", `{"a":"\u00e9\n"}`, `{"a":"é\n"}`},
		{"arrays keep their order", `[3,1,2]`, `[3,1,2]`},
	}
	for _, vector := range vectors {
		t.Run(vector.name+" [verifies SP-6~1]", func(t *testing.T) {
			got, err := format.CanonicalizeJSON([]byte(vector.input))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != vector.want {
				t.Fatalf("canonical form = %s, want %s", got, vector.want)
			}
		})
	}

	t.Run("Canonicalize serializes Go values in canonical form [verifies SP-6~1]", func(t *testing.T) {
		got, err := format.Canonicalize(map[string]any{"b": []int{2, 1}, "a": "x"})
		if err != nil {
			t.Fatal(err)
		}
		if want := `{"a":"x","b":[2,1]}`; string(got) != want {
			t.Fatalf("canonical form = %s, want %s", got, want)
		}
	})

	t.Run("invalid JSON is refused [verifies SP-6~1]", func(t *testing.T) {
		if _, err := format.CanonicalizeJSON([]byte(`{"a":`)); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("canonicalizing canonical bytes leaves them unchanged [verifies SP-6~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			once, err := format.Canonicalize(jsonValues(maxJSONDepth).Draw(t, "value"))
			if err != nil {
				t.Fatal(err)
			}
			twice, err := format.CanonicalizeJSON(once)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(once, twice) {
				t.Fatalf("not idempotent:\n%s\n%s", once, twice)
			}
		})
	})

	t.Run("canonical bytes do not depend on the order of object keys [verifies SP-7~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			value := jsonValues(maxJSONDepth).Draw(t, "value")
			reverse := func(keys []string) []string {
				reversed := make([]string, len(keys))
				for index, key := range keys {
					reversed[len(keys)-1-index] = key
				}
				return reversed
			}
			var ascending, descending bytes.Buffer
			writeWithKeyOrder(&ascending, value, func(keys []string) []string { return keys })
			writeWithKeyOrder(&descending, value, reverse)
			first, err := format.CanonicalizeJSON(ascending.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			second, err := format.CanonicalizeJSON(descending.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(first, second) {
				t.Fatalf("key order changed the canonical form:\n%s\n%s", first, second)
			}
		})
	})

	t.Run("a statement encodes to the same bytes whatever the member order of its source [verifies SP-7~1]", func(t *testing.T) {
		reordered := `{"predicateType":"p","subject":[],"_type":"t","predicate":{"b":1,"a":2}}`
		straight := `{"_type":"t","predicate":{"a":2,"b":1},"predicateType":"p","subject":[]}`
		first, err := format.CanonicalizeJSON([]byte(reordered))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(string(first), straight) {
			t.Fatalf("canonical form = %s, want %s", first, straight)
		}
	})
}
