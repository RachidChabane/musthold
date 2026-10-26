package hostproto_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

func problemPaths(t *testing.T, err error) []string {
	t.Helper()
	var invalid *hostproto.InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("error %v is not an *InvalidError", err)
	}
	paths := make([]string, len(invalid.Problems))
	for index, problem := range invalid.Problems {
		paths[index] = problem.Path
	}
	slices.Sort(paths)
	return paths
}

func TestRequestValidation(t *testing.T) {
	round := func(n int) *int { return &n }
	cases := []struct {
		name    string
		request hostproto.Request
		want    []string
	}{
		{"an open judge request", hostprototest.Request(), nil},
		{"a generation request", hostprototest.GenerationRequest(), nil},
		{"a closed request", hostprototest.ClosedRequest(), nil},
		{"a wrong type", hostprototest.Request(func(r *hostproto.Request) { r.Type = "https://rachid-chabane.com/host-protocol/request/v2" }), []string{"type"}},
		{"no module", hostprototest.Request(func(r *hostproto.Request) { r.Module = format.Module{} }), []string{"module.name", "module.version"}},
		{"an unknown kind", hostprototest.Request(func(r *hostproto.Request) { r.Kind = "judge.vibes" }), []string{"kind"}},
		{"a role that is not plugin:agent", hostprototest.Request(func(r *hostproto.Request) { r.Role = "judge" }), []string{"role"}},
		{"an agent that does not serve the kind", hostprototest.Request(func(r *hostproto.Request) { r.Role = "musthold-dp:generator" }), []string{"role"}},
		{"a closed shape on an open kind", hostprototest.Request(func(r *hostproto.Request) { r.Shape = hostproto.ShapeClosed }), []string{"closed", "shape"}},
		{"a closed request without its questions", hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Closed = nil }), []string{"closed"}},
		{"an open request with questions", hostprototest.Request(func(r *hostproto.Request) { r.Closed = json.RawMessage(`{}`) }), []string{"closed"}},
		{"an unknown shape", hostprototest.Request(func(r *hostproto.Request) { r.Shape = "wide" }), []string{"shape"}},
		{"a round on a judge request", hostprototest.Request(func(r *hostproto.Request) { r.Round = round(1) }), []string{"round"}},
		{"a generation request without a round", hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Round = nil }), []string{"round"}},
		{"a generation request with round zero", hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Round = round(0) }), []string{"round"}},
		{"a generation request without a seed", hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Seed = "" }), []string{"seed"}},
		{"no user prompt", hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "" }), []string{"prompt.user"}},
		{"a bad template digest", hostprototest.Request(func(r *hostproto.Request) { r.Prompt.TemplateDigest = "x" }), []string{"prompt.templateDigest"}},
		{"a schema that is not an object", hostprototest.Request(func(r *hostproto.Request) { r.ResponseSchema = json.RawMessage(`[1]`) }), []string{"responseSchema"}},
		{"a schema whose root is not an object type", hostprototest.Request(func(r *hostproto.Request) { r.ResponseSchema = json.RawMessage(`{"type":"array"}`) }), []string{"responseSchema.type"}},
		{"a limit of zero", hostprototest.Request(func(r *hostproto.Request) { r.Limits.MaxOutputBytes = 0 }), []string{"limits.maxOutputBytes"}},
		{"a limit above the ceiling", hostprototest.Request(func(r *hostproto.Request) { r.Limits.MaxOutputBytes = hostproto.MaxOutputBytes + 1 }), []string{"limits.maxOutputBytes"}},
		{"an unknown input role", hostprototest.Request(func(r *hostproto.Request) { r.Inputs[0].Role = "weird" }), []string{"inputs[0].role", "inputs"}},
		{"unsorted inputs", hostprototest.Request(func(r *hostproto.Request) { slices.Reverse(r.Inputs) }), []string{"inputs"}},
		{"a bad input digest", hostprototest.Request(func(r *hostproto.Request) { r.Inputs[0].Digest = "x" }), []string{"inputs[0].digest"}},
		{"a bad cache key", hostprototest.Request(func(r *hostproto.Request) { r.CacheKey = "x" }), []string{"cacheKey"}},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-117~1]", func(t *testing.T) {
			err := c.request.Validate()
			if c.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			got := problemPaths(t, err)
			want := slices.Clone(c.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("problems at %v, want %v (%v)", got, want, err)
			}
		})
	}

	t.Run("every kind has exactly one agent and the roles parse [verifies SP-117~1]", func(t *testing.T) {
		for _, kind := range hostproto.Kinds() {
			if kind.Agent() == "" {
				t.Errorf("%s has no agent", kind)
			}
			plugin, agent, err := hostproto.ParseRole(hostproto.RoleFor("musthold-dp", kind.Agent()))
			if err != nil || plugin != "musthold-dp" || agent != kind.Agent() {
				t.Errorf("%s: %q %q %v", kind, plugin, agent, err)
			}
		}
		for _, bad := range []string{"", "judge", ":judge", "p:", "p:other", "p:judge:more"} {
			if _, _, err := hostproto.ParseRole(bad); err == nil {
				t.Errorf("%q parsed as a role", bad)
			}
		}
	})

	t.Run("an executor sees the prompt, the schema and the ticket and nothing else [verifies SP-117~1]", func(t *testing.T) {
		request := hostprototest.Request()
		view := request.ViewFor(hostprototest.Ticket(request, 0, 1))
		encoded, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &members); err != nil {
			t.Fatal(err)
		}
		for _, hidden := range []string{"inputs", "cacheKey", "module", "meta", "criterion", "wave", "role", "kind", "seed", "round"} {
			if _, present := members[hidden]; present {
				t.Errorf("the executor view carries %q", hidden)
			}
		}
	})
}

func TestRequestID(t *testing.T) {
	request := hostprototest.Request()

	t.Run("the id is the SHA-256 of the JCS bytes without the module version [verifies SP-118~1]", func(t *testing.T) {
		document, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var generic map[string]any
		if err := json.Unmarshal(document, &generic); err != nil {
			t.Fatal(err)
		}
		delete(generic["module"].(map[string]any), "version")
		canonical, err := format.Canonicalize(generic)
		if err != nil {
			t.Fatal(err)
		}
		id, err := request.ID()
		if err != nil || string(id) != format.SHA256Hex(canonical) {
			t.Fatalf("id = %q (%v), want %s", id, err, format.SHA256Hex(canonical))
		}
	})

	t.Run("a module version bump keeps the id and a module name change alters it [verifies SP-118~1]", func(t *testing.T) {
		bumped := hostprototest.Request(func(r *hostproto.Request) { r.Module.Version = "9.9.9" })
		renamed := hostprototest.Request(func(r *hostproto.Request) { r.Module.Name = "musthold-alignment" })
		if hostprototest.MustID(bumped) != hostprototest.MustID(request) {
			t.Error("a version bump changed the id")
		}
		if hostprototest.MustID(renamed) == hostprototest.MustID(request) {
			t.Error("a rename kept the id")
		}
	})

	t.Run("the id is the same for any key order, including inside the response schema [verifies SP-118~1]", func(t *testing.T) {
		original := mustJSON(t, request)
		reordered := reorderKeys(t, original)
		if string(original) == string(reordered) {
			t.Fatal("the reordering changed nothing")
		}
		want, err := hostproto.RequestIDOfJSON(original)
		if err != nil {
			t.Fatal(err)
		}
		got, err := hostproto.RequestIDOfJSON(reordered)
		if err != nil || got != want {
			t.Fatalf("id = %q (%v), want %q", got, err, want)
		}
	})

	t.Run("the id is invariant to key order for any request [verifies SP-118~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			drawn := drawRequest(t)
			document := mustJSON(t, drawn)
			want, err := hostproto.RequestIDOfJSON(document)
			if err != nil {
				t.Fatal(err)
			}
			shuffled := shuffleKeys(t, document)
			if got, err := hostproto.RequestIDOfJSON(shuffled); err != nil || got != want {
				t.Fatalf("id %q (%v) after shuffling, want %q", got, err, want)
			}
		})
	})

	t.Run("the id changes with any other input [verifies SP-118~1]", func(t *testing.T) {
		mutations := map[string]func(*hostproto.Request){
			"system":   func(r *hostproto.Request) { r.Prompt.System += "!" },
			"user":     func(r *hostproto.Request) { r.Prompt.User += "!" },
			"template": func(r *hostproto.Request) { r.Prompt.TemplateDigest = hostprototest.Digest("another") },
			"schema":   func(r *hostproto.Request) { r.ResponseSchema = json.RawMessage(`{"type":"object"}`) },
			"role":     func(r *hostproto.Request) { r.Role = "musthold-alignment:judge" },
			"kind":     func(r *hostproto.Request) { r.Kind = hostproto.KindJudgeEquivalent },
			"cache":    func(r *hostproto.Request) { r.CacheKey = hostprototest.Digest("another") },
			"limits":   func(r *hostproto.Request) { r.Limits.MaxOutputBytes = 1 },
			"input":    func(r *hostproto.Request) { r.Inputs[0].Digest = hostprototest.Digest("another") },
		}
		seen := map[hostproto.RequestID]string{hostprototest.MustID(request): "base"}
		for name, mutate := range mutations {
			id := hostprototest.MustID(hostprototest.Request(mutate))
			if previous, collision := seen[id]; collision {
				t.Errorf("%s and %s share the id %s", name, previous, id)
			}
			seen[id] = name
		}
	})

	t.Run("a document that is not a request has no id [verifies SP-118~1]", func(t *testing.T) {
		for _, document := range []string{``, `[]`, `{}`, `{"module":1}`, `{"module":`} {
			if _, err := hostproto.RequestIDOfJSON([]byte(document)); !errors.Is(err, hostproto.ErrNotARequestDocument) {
				t.Errorf("%q: error = %v", document, err)
			}
		}
	})

	t.Run("the schema digest follows the canonical schema [verifies SP-118~1]", func(t *testing.T) {
		spaced := hostprototest.Request(func(r *hostproto.Request) {
			r.ResponseSchema = json.RawMessage(strings.ReplaceAll(hostprototest.ResponseSchema, ":", " : "))
		})
		a, errA := request.SchemaDigest()
		b, errB := spaced.SchemaDigest()
		if errA != nil || errB != nil || a != b {
			t.Fatalf("digests %q %q (%v %v)", a, b, errA, errB)
		}
	})
}

// fataler is what the helpers need of a *testing.T or a *rapid.T.
type fataler interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func mustJSON(t fataler, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// reorderKeys rewrites every object of a document with its members in reverse
// alphabetical order.
func reorderKeys(t fataler, document []byte) []byte {
	t.Helper()
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	writeOrdered(t, &out, value, func(keys []string) { slices.Sort(keys); slices.Reverse(keys) })
	return []byte(out.String())
}

func writeOrdered(t fataler, out *strings.Builder, value any, order func([]string)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		order(keys)
		out.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				out.WriteByte(',')
			}
			out.Write(mustJSON(t, key))
			out.WriteByte(':')
			writeOrdered(t, out, typed[key], order)
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			writeOrdered(t, out, item, order)
		}
		out.WriteByte(']')
	default:
		out.Write(mustJSON(t, typed))
	}
}

func shuffleKeys(t *rapid.T, document []byte) []byte {
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	writeOrdered(t, &out, value, func(keys []string) {
		slices.Sort(keys)
		permutation := rapid.Permutation(keys).Draw(t, "order")
		copy(keys, permutation)
	})
	return []byte(out.String())
}

func drawRequest(t *rapid.T) hostproto.Request {
	properties := rapid.MapOfN(rapid.StringMatching(`[a-z]{1,6}`), rapid.SampledFrom([]string{"a", "b", "c"}), 1, 4).Draw(t, "properties")
	schema := mustJSON(t, map[string]any{"type": "object", "properties": properties, "additionalProperties": false})
	return hostprototest.Request(func(r *hostproto.Request) {
		r.Prompt.User = rapid.StringN(1, 40, -1).Draw(t, "user")
		r.Prompt.System = rapid.String().Draw(t, "system")
		r.ResponseSchema = schema
		r.CacheKey = hostprototest.Digest(rapid.String().Draw(t, "cache"))
	})
}

func TestTickets(t *testing.T) {
	request := hostprototest.Request()
	id := hostproto.TicketID{RequestID: hostprototest.MustID(request), Vote: 2, Emission: 3}

	t.Run("a ticket identifier round-trips through its text form [verifies SP-119~1]", func(t *testing.T) {
		parsed, err := hostproto.ParseTicketID(id.String())
		if err != nil || parsed != id {
			t.Fatalf("parsed %+v (%v), want %+v", parsed, err, id)
		}
	})

	t.Run("text that is not an identifier is refused [verifies SP-119~1]", func(t *testing.T) {
		bad := []string{
			"", "x", id.String() + ".1", strings.TrimSuffix(id.String(), ".3"),
			strings.ToUpper(id.String()), string(id.RequestID) + ".-1.1", string(id.RequestID) + ".0.0",
			string(id.RequestID) + ".01.1", string(id.RequestID) + ".a.1", string(id.RequestID)[1:] + ".0.1",
		}
		for _, text := range bad {
			if _, err := hostproto.ParseTicketID(text); !errors.Is(err, hostproto.ErrBadTicket) {
				t.Errorf("%q: error = %v", text, err)
			}
		}
	})

	t.Run("a nonce is 128 bits drawn from the given reader [verifies SP-119~1]", func(t *testing.T) {
		ticket, err := hostproto.NewTicket(id, strings.NewReader(strings.Repeat("\xab", hostproto.NonceBytes)))
		if err != nil {
			t.Fatal(err)
		}
		if ticket.Nonce != strings.Repeat("ab", hostproto.NonceBytes) || !hostproto.ValidNonce(ticket.Nonce) {
			t.Fatalf("nonce = %q", ticket.Nonce)
		}
		if _, err := hostproto.NewTicket(id, strings.NewReader("short")); err == nil {
			t.Error("a short entropy source was accepted")
		}
		if _, err := hostproto.NewTicket(hostproto.TicketID{}, strings.NewReader(strings.Repeat("a", 16))); err == nil {
			t.Error("an invalid identifier was accepted")
		}
	})

	t.Run("the nonce stays outside the request id [verifies SP-119~1]", func(t *testing.T) {
		first, second := hostprototest.Ticket(request, 0, 1), hostprototest.Ticket(request, 0, 1)
		first.Nonce = strings.Repeat("0", hostproto.NonceHexLength)
		if first.RequestID != second.RequestID {
			t.Fatal("the nonce reached the request id")
		}
	})

	t.Run("a prompt holds exactly one ticket [verifies SP-119~1]", func(t *testing.T) {
		other := hostproto.TicketID{RequestID: id.RequestID, Vote: 0, Emission: 1}
		cases := []struct {
			name string
			text string
			ok   bool
		}{
			{"the bare ticket", id.String(), true},
			{"the executor prompt", hostproto.ExecutorPrompt(id), true},
			{"a padded prompt", "context " + id.String() + " more", true},
			{"no ticket", "ticket none", false},
			{"two tickets", id.String() + " " + other.String(), false},
			{"the same ticket twice", id.String() + " " + id.String(), false},
			{"a ticket glued to a word", "x" + id.String(), false},
		}
		for _, c := range cases {
			got, err := hostproto.FindTicketID(c.text)
			if c.ok && (err != nil || got != id) {
				t.Errorf("%s: %+v, %v", c.name, got, err)
			}
			if !c.ok && !errors.Is(err, hostproto.ErrNoSingleTicket) {
				t.Errorf("%s: error = %v", c.name, err)
			}
		}
	})

	t.Run("a ticket validates its parts [verifies SP-119~1]", func(t *testing.T) {
		for _, bad := range []hostproto.TicketID{
			{RequestID: "x", Emission: 1}, {RequestID: id.RequestID, Vote: -1, Emission: 1}, {RequestID: id.RequestID, Emission: 0},
		} {
			if bad.Validate() == nil {
				t.Errorf("%+v validated", bad)
			}
		}
		if id.Validate() != nil || id.Key() != (hostproto.Key{RequestID: id.RequestID, Vote: 2}) {
			t.Error("a valid ticket was refused or keyed wrongly")
		}
	})

	t.Run("a response validates its parts [verifies SP-119~1]", func(t *testing.T) {
		valid := hostprototest.Response(hostprototest.Ticket(request, 0, 1))
		if err := valid.Validate(); err != nil {
			t.Fatal(err)
		}
		mutations := map[string]func(*hostproto.Response){
			"requestId": func(r *hostproto.Response) { r.RequestID = "x" },
			"vote":      func(r *hostproto.Response) { r.Vote = -1 },
			"nonce":     func(r *hostproto.Response) { r.Nonce = "short" },
			"output":    func(r *hostproto.Response) { r.Output = json.RawMessage(`{`) },
			"array":     func(r *hostproto.Response) { r.Output = json.RawMessage(`[1]`) },
			"null":      func(r *hostproto.Response) { r.Output = json.RawMessage(`null`) },
			"oversize": func(r *hostproto.Response) {
				r.Output = json.RawMessage(`"` + strings.Repeat("a", hostproto.MaxOutputBytes) + `"`)
			},
			"executor": func(r *hostproto.Response) { r.Executor.Channel = "" },
		}
		for name, mutate := range mutations {
			broken := valid
			mutate(&broken)
			if broken.Validate() == nil {
				t.Errorf("%s: a broken response validated", name)
			}
		}
	})
}
