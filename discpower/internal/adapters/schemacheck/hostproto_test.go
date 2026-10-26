package schemacheck

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

func compileHostProtocol(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	compiled, err := compileAll(append(format.Schemas(), hostproto.Schemas()...))
	if err != nil {
		t.Fatalf("compile schemas: %v", err)
	}
	return compiled
}

func schemaAccepts(t *testing.T, schema *jsonschema.Schema, document any) bool {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	return schema.Validate(instance) == nil
}

func TestHostProtocolSchemas(t *testing.T) {
	compiled := compileHostProtocol(t)

	t.Run("a request is accepted by its schema exactly when its domain rules hold [verifies SP-146~1]", func(t *testing.T) {
		schema := compiled[hostproto.RequestType]
		round := 2
		mutations := map[string]func(*hostproto.Request){
			"valid":                 func(*hostproto.Request) {},
			"a wrong type":          func(r *hostproto.Request) { r.Type = "https://example.com/v2" },
			"no module name":        func(r *hostproto.Request) { r.Module.Name = "" },
			"no module version":     func(r *hostproto.Request) { r.Module.Version = "" },
			"an unknown kind":       func(r *hostproto.Request) { r.Kind = "judge.vibes" },
			"a role without agent":  func(r *hostproto.Request) { r.Role = "judge" },
			"another agent's role":  func(r *hostproto.Request) { r.Role = "musthold-dp:generator" },
			"an open kind, closed":  func(r *hostproto.Request) { r.Shape = hostproto.ShapeClosed },
			"an unknown shape":      func(r *hostproto.Request) { r.Shape = "wide" },
			"questions on an open":  func(r *hostproto.Request) { r.Closed = json.RawMessage(`{}`) },
			"a round on a judgment": func(r *hostproto.Request) { r.Round = &round },
			"a seed on a judgment":  func(r *hostproto.Request) { r.Seed = hostprototest.Digest("seed") },
			"no user prompt":        func(r *hostproto.Request) { r.Prompt.User = "" },
			"a bad template digest": func(r *hostproto.Request) { r.Prompt.TemplateDigest = "x" },
			"an array schema":       func(r *hostproto.Request) { r.ResponseSchema = json.RawMessage(`{"type":"array"}`) },
			"no output limit":       func(r *hostproto.Request) { r.Limits.MaxOutputBytes = 0 },
			"a limit above ceiling": func(r *hostproto.Request) { r.Limits.MaxOutputBytes = hostproto.MaxOutputBytes + 1 },
			"an unknown input role": func(r *hostproto.Request) { r.Inputs[0].Role = "weird" },
			"a bad input digest":    func(r *hostproto.Request) { r.Inputs[0].Digest = "x" },
			"a bad cache key":       func(r *hostproto.Request) { r.CacheKey = strings.ToUpper(hostprototest.Digest("k")) },
			"a host response input": func(r *hostproto.Request) { r.Inputs[0].Role = format.RoleHostResponse },
			"a proof input":         func(r *hostproto.Request) { r.Inputs[0].Role = format.RoleProof },
		}
		for name, mutate := range mutations {
			request := hostprototest.Request(mutate)
			if got, want := schemaAccepts(t, schema, request), request.Validate() == nil; got != want {
				t.Errorf("%s: schema accepts = %v, domain accepts = %v", name, got, want)
			}
		}
	})

	t.Run("generation and closed requests follow their own rules in both [verifies SP-146~1]", func(t *testing.T) {
		schema := compiled[hostproto.RequestType]
		round := 1
		cases := map[string]hostproto.Request{
			"a generation request":    hostprototest.GenerationRequest(),
			"no round":                hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Round = nil }),
			"round zero":              hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Round = new(int) }),
			"no seed":                 hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Seed = "" }),
			"a judge on a generation": hostprototest.GenerationRequest(func(r *hostproto.Request) { r.Role = "musthold-dp:judge" }),
			"a closed request":        hostprototest.ClosedRequest(),
			"no questions":            hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Closed = nil }),
			"an open shape on closed": hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Shape = hostproto.ShapeOpen }),
			"a round on a closed one": hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Round = &round }),
			"a judge deciding closed": hostprototest.ClosedRequest(func(r *hostproto.Request) { r.Role = "musthold-dp:judge" }),
		}
		for name, request := range cases {
			if got, want := schemaAccepts(t, schema, request), request.Validate() == nil; got != want {
				t.Errorf("%s: schema accepts = %v, domain accepts = %v", name, got, want)
			}
		}
	})

	t.Run("a ticket and a response are accepted by their schemas exactly when their domain rules hold [verifies SP-146~1]", func(t *testing.T) {
		request := hostprototest.Request()
		ticket := hostprototest.Ticket(request, 0, 1)
		tickets := map[string]hostproto.Ticket{
			"valid":           ticket,
			"a bad nonce":     {TicketID: ticket.TicketID, Nonce: "x"},
			"a bad request":   {TicketID: hostproto.TicketID{RequestID: "x", Emission: 1}, Nonce: ticket.Nonce},
			"emission zero":   {TicketID: hostproto.TicketID{RequestID: ticket.RequestID}, Nonce: ticket.Nonce},
			"a negative vote": {TicketID: hostproto.TicketID{RequestID: ticket.RequestID, Vote: -1, Emission: 1}, Nonce: ticket.Nonce},
		}
		for name, candidate := range tickets {
			if got, want := schemaAccepts(t, compiled[hostproto.TicketType], candidate), candidate.Validate() == nil; got != want {
				t.Errorf("ticket %s: schema accepts = %v, domain accepts = %v", name, got, want)
			}
		}
		valid := hostprototest.Response(ticket)
		responses := map[string]func(*hostproto.Response){
			"valid":              func(*hostproto.Response) {},
			"a bad nonce":        func(r *hostproto.Response) { r.Nonce = "x" },
			"an array output":    func(r *hostproto.Response) { r.Output = json.RawMessage(`[1]`) },
			"a null output":      func(r *hostproto.Response) { r.Output = json.RawMessage(`null`) },
			"an unknown channel": func(r *hostproto.Response) { r.Executor.Channel = "carrier-pigeon" },
			"an unnamed harness": func(r *hostproto.Response) { r.Executor.Harness.Name = "" },
			"an attested executor without attestation": func(r *hostproto.Response) {
				r.Executor.Isolation = format.IsolationRestrictedAttested
			},
		}
		for name, mutate := range responses {
			response := valid
			mutate(&response)
			if got, want := schemaAccepts(t, compiled[hostproto.ResponseType], response), response.Validate() == nil; got != want {
				t.Errorf("response %s: schema accepts = %v, domain accepts = %v", name, got, want)
			}
		}
	})
}
