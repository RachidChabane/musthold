package format_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/format"
)

func TestSystemOneEndpointSource(t *testing.T) {
	const digest = "0123abcd"

	t.Run("a relay source is system-one, the model, an at sign and the endpoint digest [verifies SP-960~1]", func(t *testing.T) {
		if got := format.SystemOneEndpointSource("tev1:0.8b", digest); got != "system-one:tev1:0.8b@0123abcd" {
			t.Fatalf("source %q", got)
		}
	})

	t.Run("a source a relay imports is named unchanged and is a known source [verifies SP-960~1]", func(t *testing.T) {
		for _, text := range []string{"system-one:nimble@0123abcd", "system-one:tev1:0.8b@0123abcd", "system-one:a@b@c"} {
			got, err := format.ParseSystemOneSource(text)
			if err != nil || string(got) != text || !got.IsKnown() {
				t.Errorf("ParseSystemOneSource(%q) = %q, %v", text, got, err)
			}
		}
		if parsed, _ := format.ParseSystemOneSource("system-one:m@d"); parsed != format.SystemOneEndpointSource("m", "d") {
			t.Errorf("parsed %q differs from the built source", parsed)
		}
	})

	t.Run("any other text is refused, the relay's former spelling included [verifies SP-960~1]", func(t *testing.T) {
		for _, text := range []string{
			"", "system-one", "system-one:", "system-one:@d", "system-one:m@", "system-one:m",
			"systemone:m@d", "host-votes(3)", "execution",
		} {
			if got, err := format.ParseSystemOneSource(text); !errors.Is(err, format.ErrSystemOneSource) || got != "" {
				t.Errorf("ParseSystemOneSource(%q) = %q, %v", text, got, err)
			}
		}
	})
}
