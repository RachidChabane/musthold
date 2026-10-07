// Command calibration-decode reads the recorded reply of a `/v1/systemone` endpoint the way the relay and the
// proof module would, for the calibration experiment:
//
//	decode           the relay's checks (systemonewire.DecodeClosed, DecodeReply and question.Set.Check), then
//	                 the module import (hostproto.ClosedSet.AnswersFromSystemOne) and the module reading
//	                 (hostproto.ParseClosedAnswers);
//	endpoint-digest  the digest and the source string the relay builds for an endpoint address.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/systemonewire"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/endpoint"
)

const (
	commandDecode         = "decode"
	commandEndpointDigest = "endpoint-digest"
	exitUsage             = 2
	exitFailure           = 1
	statusOK              = "ok"
	statusRefused         = "relay_refused"
	stageClosed           = "closed"
	stageReply            = "reply"
	stageCheck            = "check"
	stageImport           = "import"
	stageRead             = "read"
)

type decodeRequest struct {
	Key    string `json:"key"`
	Closed string `json:"closed"`
	Reply  string `json:"reply"`
}

type answerOutput struct {
	ID            string             `json:"id"`
	Choice        string             `json:"choice"`
	Confidence    *float64           `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

type decodeResult struct {
	Key      string         `json:"key"`
	Status   string         `json:"status"`
	Stage    string         `json:"stage,omitempty"`
	Error    string         `json:"error,omitempty"`
	Model    string         `json:"model,omitempty"`
	Document string         `json:"document,omitempty"`
	Answers  []answerOutput `json:"answers,omitempty"`
}

type digestOutput struct {
	Address string `json:"address"`
	Digest  string `json:"digest"`
	Source  string `json:"source"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: calibration-decode decode | endpoint-digest --address URL --model NAME")
		os.Exit(exitUsage)
	}
	var err error
	switch os.Args[1] {
	case commandDecode:
		err = runDecode(os.Stdin, os.Stdout)
	case commandEndpointDigest:
		err = runEndpointDigest(os.Args[2:], os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "calibration-decode: unknown command %q\n", os.Args[1])
		os.Exit(exitUsage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibration-decode: %v\n", err)
		os.Exit(exitFailure)
	}
}

func runEndpointDigest(args []string, out io.Writer) error {
	flags := flag.NewFlagSet(commandEndpointDigest, flag.ContinueOnError)
	address := flags.String("address", "", "the endpoint address")
	model := flags.String("model", "", "the model the reply names")
	if err := flags.Parse(args); err != nil {
		return err
	}
	parsed, err := endpoint.Parse(*address)
	if err != nil {
		return err
	}
	return writeJSON(out, digestOutput{Address: parsed.URL(), Digest: parsed.Digest(), Source: parsed.Source(*model)})
}

func runDecode(in io.Reader, out io.Writer) error {
	var requests []decodeRequest
	if err := json.NewDecoder(in).Decode(&requests); err != nil {
		return fmt.Errorf("read the requests: %w", err)
	}
	results := make([]decodeResult, 0, len(requests))
	for _, request := range requests {
		results = append(results, decode(request))
	}
	return writeJSON(out, results)
}

func refused(key, stage string, err error) decodeResult {
	return decodeResult{Key: key, Status: statusRefused, Stage: stage, Error: err.Error()}
}

func decode(request decodeRequest) decodeResult {
	set, err := systemonewire.DecodeClosed(json.RawMessage(request.Closed))
	if err != nil {
		return refused(request.Key, stageClosed, err)
	}
	answers, model, err := systemonewire.DecodeReply([]byte(request.Reply), set)
	if err != nil {
		return refused(request.Key, stageReply, err)
	}
	if err := set.Check(answers); err != nil {
		return refused(request.Key, stageCheck, err)
	}
	closed, err := hostproto.ParseClosedSet(json.RawMessage(request.Closed))
	if err != nil {
		return refused(request.Key, stageClosed, err)
	}
	document, err := closed.AnswersFromSystemOne([]byte(request.Reply))
	if err != nil {
		return refused(request.Key, stageImport, err)
	}
	parsed, err := hostproto.ParseClosedAnswers(document)
	if err != nil {
		return refused(request.Key, stageRead, err)
	}
	result := decodeResult{Key: request.Key, Status: statusOK, Model: model, Document: string(document)}
	for _, q := range closed.Questions {
		answer, found := parsed[q.ID]
		if !found {
			return refused(request.Key, stageRead, errors.New("the imported document does not answer "+q.ID))
		}
		output := answerOutput{ID: q.ID, Choice: answer.Choice, Probabilities: answer.Probabilities}
		if answer.HasConfidence {
			confidence := answer.Confidence
			output.Confidence = &confidence
		}
		result.Answers = append(result.Answers, output)
	}
	return result
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
