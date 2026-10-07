package systemone_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/systemone"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/failure"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
	"github.com/RachidChabane/spec-proof/s1relay/internal/testkit/stubendpoint"
)

const (
	recordedModel = "tev1:0.8b"
	shortTimeout  = 300 * time.Millisecond
	testTimeout   = 5 * time.Second
)

func skySet() question.Set {
	return question.Set{
		State: "the sky is green",
		Questions: []question.Question{{
			ID: "color", Type: question.TypeChoice, Text: "Which color does the text say the sky is?",
			Options: []question.Option{{Name: "blue"}, {Name: "green"}, {Name: "grey"}},
		}},
	}
}

func clientFor(t *testing.T, stub *stubendpoint.Server, model string, timeout time.Duration) *systemone.Client {
	t.Helper()
	address, err := endpoint.Parse(stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	return systemone.NewClient(address, model, timeout)
}

func fixed(status int, body string) stubendpoint.Responder {
	return func(int, []byte) stubendpoint.Reply { return stubendpoint.Reply{Status: status, Body: body} }
}

func TestDecideAgainstRecordedServers(t *testing.T) {
	t.Run("an Ollama server receives the recorded request and its reply gives the recorded answer [verifies SP-605~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusOK, stubendpoint.RecordedOllamaChoiceReply))
		defer stub.Close()
		decision, err := clientFor(t, stub, recordedModel, testTimeout).Decide(context.Background(), skySet())
		if err != nil {
			t.Fatal(err)
		}
		if got := stub.Requests(); len(got) != 1 || string(got[0]) != stubendpoint.RecordedOllamaChoiceRequest {
			t.Fatalf("requests %q", got)
		}
		if decision.Model != recordedModel || decision.Answers[0].Choice != "green" || *decision.Answers[0].Confidence != 0.9674686077174837 {
			t.Fatalf("decision %+v", decision)
		}
	})

	t.Run("a Kev server answers with the model it was called by when none is configured [verifies SP-605~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, fixed(http.StatusOK, stubendpoint.RecordedKevChoiceReply))
		defer stub.Close()
		decision, err := clientFor(t, stub, "", testTimeout).Decide(context.Background(), skySet())
		if err != nil {
			t.Fatal(err)
		}
		if decision.Model != "kev-latest" || decision.Answers[0].Choice != "green" {
			t.Fatalf("decision %+v", decision)
		}
		if body := stub.Requests()[0]; strings.Contains(string(body), `"model"`) {
			t.Fatalf("request %s names a model", body)
		}
	})

	t.Run("the default responder answers every question of a request with its first option [verifies SP-604~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		decision, err := clientFor(t, stub, recordedModel, testTimeout).Decide(context.Background(), skySet())
		if err != nil || decision.Answers[0].Choice != "blue" {
			t.Fatalf("decision %+v (%v)", decision, err)
		}
	})
}

func TestDecideFailures(t *testing.T) {
	cases := []struct {
		name    string
		stub    func() *stubendpoint.Server
		want    failure.Kind
		mention string
	}{
		{"a state too large for the model is refused with 400", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusBadRequest, `{"error":"state exceeds the context"}`))
		}, failure.KindRejected, "HTTP 400: {\"error\":\"state exceeds the context\"}"},
		{"an unknown route is a 404", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusNotFound, "404 page not found"))
		}, failure.KindRejected, "HTTP 404"},
		{"a failing server is a 5xx", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Kev, fixed(http.StatusInternalServerError, "model crashed"))
		}, failure.KindServer, "HTTP 500: model crashed"},
		{"a redirect is not followed", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Kev, fixed(http.StatusFound, ""))
		}, failure.KindRejected, "unexpected HTTP 302"},
		{"a reply that is not JSON", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusOK, "<html>proxy error</html>"))
		}, failure.KindMalformed, "unusable"},
		{"a reply that answers another question", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusOK, `{"model":"m","answers":{"other":{"type":"choice","choice":"green"}}}`))
		}, failure.KindMalformed, "color"},
		{"a reply that hangs", func() *stubendpoint.Server {
			return stubendpoint.New(stubendpoint.Ollama, func(int, []byte) stubendpoint.Reply { return stubendpoint.Reply{Hang: true} })
		}, failure.KindTimeout, "no reply in time"},
	}
	for _, c := range cases {
		t.Run(c.name+" leaves the request unanswered with its kind [verifies SP-607~1]", func(t *testing.T) {
			stub := c.stub()
			defer stub.Close()
			_, err := clientFor(t, stub, recordedModel, shortTimeout).Decide(context.Background(), skySet())
			if err == nil || failure.KindOf(err) != c.want {
				t.Fatalf("error %v, want kind %s", err, c.want)
			}
			if !strings.Contains(failure.DetailOf(err), c.mention) {
				t.Errorf("detail %q does not mention %q", failure.DetailOf(err), c.mention)
			}
		})
	}

	t.Run("a server that is not listening is a transport failure [verifies SP-607~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		client := clientFor(t, stub, recordedModel, shortTimeout)
		stub.Close()
		if _, err := client.Decide(context.Background(), skySet()); failure.KindOf(err) != failure.KindTransport {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a question set that cannot be sent is unsupported and posts nothing [verifies SP-616~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		_, err := clientFor(t, stub, recordedModel, shortTimeout).Decide(context.Background(), question.Set{})
		if failure.KindOf(err) != failure.KindUnsupported || len(stub.Requests()) != 0 {
			t.Fatalf("error %v, %d requests", err, len(stub.Requests()))
		}
	})

	t.Run("the excerpt of a long error body is cut [verifies SP-607~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, fixed(http.StatusBadRequest, strings.Repeat("x", 5000)))
		defer stub.Close()
		_, err := clientFor(t, stub, recordedModel, testTimeout).Decide(context.Background(), skySet())
		if detail := failure.DetailOf(err); len(detail) > 400 || !strings.HasSuffix(detail, "...") {
			t.Fatalf("detail has %d bytes", len(detail))
		}
	})
}

func TestUnload(t *testing.T) {
	address := func(stub *stubendpoint.Server) endpoint.Endpoint {
		parsed, err := endpoint.Parse(stub.URL)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}

	t.Run("an Ollama server gets a generate request with keep_alive zero for the model [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		result, err := systemone.NewOllamaUnloader(address(stub), "", testTimeout).Unload(context.Background(), recordedModel)
		if err != nil || !result.Applicable {
			t.Fatalf("result %+v (%v)", result, err)
		}
		if got := stub.Unloads(); len(got) != 1 || string(got[0]) != `{"model":"tev1:0.8b","keep_alive":0}` {
			t.Fatalf("unloads %q", got)
		}
	})

	t.Run("the configured model is unloaded when the batch learned none [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		if _, err := systemone.NewOllamaUnloader(address(stub), "configured:1b", testTimeout).Unload(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
		if got := stub.Unloads(); len(got) != 1 || !strings.Contains(string(got[0]), "configured:1b") {
			t.Fatalf("unloads %q", got)
		}
	})

	t.Run("no model name means no unload [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		result, err := systemone.NewOllamaUnloader(address(stub), "", testTimeout).Unload(context.Background(), "")
		if err != nil || result.Applicable || result.Detail == "" || len(stub.Unloads()) != 0 {
			t.Fatalf("result %+v (%v), %d unloads", result, err, len(stub.Unloads()))
		}
	})

	t.Run("a Kev server has no version route and is left untouched [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		result, err := systemone.NewOllamaUnloader(address(stub), "", testTimeout).Unload(context.Background(), "kev-latest")
		if err != nil || result.Applicable || len(stub.Unloads()) != 0 {
			t.Fatalf("result %+v (%v)", result, err)
		}
	})

	t.Run("a refused unload and an unreachable server are errors for the caller to report [verifies SP-611~1]", func(t *testing.T) {
		refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/version" {
				w.Write([]byte(`{"version":"0.35.1"}`))
				return
			}
			http.Error(w, "no", http.StatusInternalServerError)
		}))
		defer refusing.Close()
		refusingAddress, err := endpoint.Parse(refusing.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := systemone.NewOllamaUnloader(refusingAddress, "m", testTimeout).Unload(context.Background(), "m"); err == nil {
			t.Fatal("a refused unload is not an error")
		}
		gone := stubendpoint.New(stubendpoint.Ollama, nil)
		target := address(gone)
		gone.Close()
		if _, err := systemone.NewOllamaUnloader(target, "m", shortTimeout).Unload(context.Background(), "m"); err == nil {
			t.Fatal("an unreachable server is not an error")
		}
	})
}
