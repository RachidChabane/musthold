// Package stubendpoint is a System One endpoint for tests, in two flavours that
// reproduce the replies recorded from Ollama 0.35.1 and from a Kev server. It is
// never linked into the binary and never reaches a real model.
package stubendpoint

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
)

// Flavor selects which server the stub imitates.
type Flavor int

// The two servers of the recorded findings.
const (
	// Ollama serves `/v1/systemone`, `/api/version` and `/api/generate`.
	Ollama Flavor = iota
	// Kev serves `/v1/systemone` only and adds `latency_ms` to its replies.
	Kev
)

// Version is the version the Ollama flavour reports.
const Version = "0.35.1"

const (
	routeSystemOne = "/v1/systemone"
	routeVersion   = "/api/version"
	routeGenerate  = "/api/generate"
	contentType    = "Content-Type"
	mediaJSON      = "application/json"
	kevLatencyMs   = 29.9
	statedChoice   = 0.9674686077174837
)

// Reply is what the stub answers to one closed-question request.
type Reply struct {
	Status int
	Body   string
	// Hang makes the stub hold the request until the client gives up.
	Hang bool
}

// Responder decides the reply to the index-th closed-question request, from its
// body. A nil Responder answers every question with its first option.
type Responder func(index int, body []byte) Reply

// Server is a running stub.
type Server struct {
	*httptest.Server
	// UnloadStatus is the status of a generate request; zero means 200.
	UnloadStatus int
	flavor       Flavor
	respond      Responder

	mutex       sync.Mutex
	requests    [][]byte
	unloads     [][]byte
	inFlight    int
	maxInFlight int
}

// New starts a stub of the flavour. Close it with the embedded Server's Close.
func New(flavor Flavor, respond Responder) *Server {
	stub := &Server{flavor: flavor, respond: respond}
	stub.Server = httptest.NewServer(http.HandlerFunc(stub.serve))
	return stub
}

// Requests are the bodies of the closed-question requests received, in order.
func (s *Server) Requests() [][]byte {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([][]byte(nil), s.requests...)
}

// Unloads are the bodies of the generate requests received, in order.
func (s *Server) Unloads() [][]byte {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([][]byte(nil), s.unloads...)
}

// MaxInFlight is the most closed-question requests that were open at once.
func (s *Server) MaxInFlight() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.maxInFlight
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == routeSystemOne && r.Method == http.MethodPost:
		s.serveQuestions(w, r)
	case s.flavor == Ollama && r.URL.Path == routeVersion && r.Method == http.MethodGet:
		w.Header().Set(contentType, mediaJSON)
		fmt.Fprintf(w, `{"version":%q}`, Version)
	case s.flavor == Ollama && r.URL.Path == routeGenerate && r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		s.mutex.Lock()
		s.unloads = append(s.unloads, body)
		s.mutex.Unlock()
		if s.UnloadStatus != 0 {
			http.Error(w, "unload refused", s.UnloadStatus)
			return
		}
		w.Header().Set(contentType, mediaJSON)
		w.Write([]byte(`{"done":true}`))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveQuestions(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mutex.Lock()
	index := len(s.requests)
	s.requests = append(s.requests, body)
	s.inFlight++
	s.maxInFlight = max(s.maxInFlight, s.inFlight)
	s.mutex.Unlock()
	defer func() {
		s.mutex.Lock()
		s.inFlight--
		s.mutex.Unlock()
	}()
	reply := s.replyTo(index, body)
	if reply.Hang {
		<-r.Context().Done()
		return
	}
	w.Header().Set(contentType, mediaJSON)
	w.WriteHeader(reply.Status)
	w.Write([]byte(reply.Body))
}

func (s *Server) replyTo(index int, body []byte) Reply {
	if s.respond != nil {
		return s.respond(index, body)
	}
	return Reply{Status: http.StatusOK, Body: s.Answer(body)}
}

type wireQuestion struct {
	Type     string          `json:"type"`
	Criteria json.RawMessage `json:"criteria"`
}

// Answer builds the reply of the flavour that answers every question of a request
// body: a choice with its first option in name order, a score with its top level.
func (s *Server) Answer(body []byte) string {
	var request struct {
		Model     string                  `json:"model"`
		Questions map[string]wireQuestion `json:"questions"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "{}"
	}
	answers := map[string]any{}
	for name, question := range request.Questions {
		answers[name] = answerTo(question)
	}
	reply := map[string]any{"model": request.Model, "answers": answers}
	if s.flavor == Kev {
		reply["usage"] = map[string]int{"input_tokens": 32, "output_tokens": 63}
		reply["latency_ms"] = kevLatencyMs
	} else {
		reply["usage"] = map[string]int{"input_tokens": 125, "output_tokens": 1}
	}
	encoded, _ := json.Marshal(reply)
	return string(encoded)
}

func answerTo(question wireQuestion) map[string]any {
	if question.Type == "score" {
		var levels []string
		_ = json.Unmarshal(question.Criteria, &levels)
		return map[string]any{"type": "score", "score": float64(len(levels) - 1), "legend": levels, "confidence": statedChoice}
	}
	var options map[string]string
	_ = json.Unmarshal(question.Criteria, &options)
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names)
	probabilities := map[string]float64{}
	for _, name := range names {
		probabilities[name] = 1 / float64(len(names))
	}
	return map[string]any{"type": "choice", "choice": names[0], "probabilities": probabilities, "confidence": statedChoice}
}
