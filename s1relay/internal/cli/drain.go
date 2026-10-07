package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/memguard"
	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/modulecli"
	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/projectconfig"
	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/systemone"
	"github.com/RachidChabane/spec-proof/s1relay/internal/app/drain"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/endpoint"
)

// Flags of `drain`.
const (
	flagModule      = "module"
	flagEndpoint    = "endpoint"
	flagModel       = "model"
	flagTimeout     = "timeout"
	flagMaxRequests = "max-requests"
	flagRoot        = "root"
	flagMemGuard    = "mem-guard"
	flagNoUnload    = "no-unload"
)

// drainSettings are the settings of one drain after the flags and the project
// configuration are merged; a flag wins over the configuration.
type drainSettings struct {
	module      string
	endpoint    endpoint.Endpoint
	model       string
	timeout     time.Duration
	maxRequests int
	root        string
	memGuard    bool
	unload      bool
}

type drainReport struct {
	Module         string       `json:"module"`
	EndpointDigest string       `json:"endpointDigest"`
	Model          string       `json:"model,omitempty"`
	Answered       int          `json:"answered"`
	Unanswered     int          `json:"unanswered"`
	Remaining      int          `json:"remaining"`
	Items          []reportItem `json:"items"`
	Unload         reportUnload `json:"unload"`
}

type reportItem struct {
	Ticket string `json:"ticket"`
	Status string `json:"status"`
	Source string `json:"source,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type reportUnload struct {
	Attempted bool   `json:"attempted"`
	Done      bool   `json:"done"`
	Detail    string `json:"detail,omitempty"`
}

// logWriter writes the progress lines of a drain to stderr.
type logWriter struct {
	out    io.Writer
	binary string
}

func (l logWriter) Logf(format string, args ...any) {
	fmt.Fprintf(l.out, "%s: %s\n", l.binary, fmt.Sprintf(format, args...))
}

func (e Env) drain(args []string) int {
	settings, err := e.settingsFrom(args)
	if err != nil {
		fmt.Fprintf(e.Stderr, "%s: %v\n", e.Binary, err)
		return ExitToolFailure
	}
	gateway, err := modulecli.NewGateway(settings.module, settings.root, e.Runner)
	if err != nil {
		fmt.Fprintf(e.Stderr, "%s: %v\n", e.Binary, err)
		return ExitToolFailure
	}
	service := drain.Service{
		Module:    gateway,
		Endpoint:  settings.endpoint,
		SystemOne: systemone.NewClient(settings.endpoint, settings.model, settings.timeout),
		Unloader:  systemone.NewOllamaUnloader(settings.endpoint, settings.model, settings.timeout),
		Guard:     e.NewGuard(e.minFreePercent()),
		Log:       logWriter{out: e.Stderr, binary: e.Binary},
	}
	report, err := service.Run(context.Background(), drain.Options{MaxRequests: settings.maxRequests, MemGuard: settings.memGuard, Unload: settings.unload})
	if err != nil {
		fmt.Fprintf(e.Stderr, "%s: %v\n", e.Binary, err)
		if errors.Is(err, drain.ErrMemoryRefused) {
			return ExitMemoryRefused
		}
		return ExitToolFailure
	}
	if err := json.NewEncoder(e.Stdout).Encode(presentReport(settings, report)); err != nil {
		fmt.Fprintf(e.Stderr, "%s: %v\n", e.Binary, err)
		return ExitToolFailure
	}
	if report.Unanswered() > 0 {
		return ExitUnanswered
	}
	return ExitClean
}

func (e Env) settingsFrom(args []string) (drainSettings, error) {
	set := flag.NewFlagSet(e.Binary+" "+drainCommand, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	module := set.String(flagModule, "", "the module whose closed requests are relayed, for example dp")
	address := set.String(flagEndpoint, "", "the System One endpoint (default: systemone.endpoint of the project configuration)")
	model := set.String(flagModel, "", "the model to name in each request (default: systemone.model)")
	timeout := set.Duration(flagTimeout, systemone.DefaultTimeout, "the time limit of one request")
	maxRequests := set.Int(flagMaxRequests, drain.Unlimited, "the most requests to post; 0 posts them all")
	root := set.String(flagRoot, "", "the project root (default: the working directory)")
	memGuard := set.Bool(flagMemGuard, false, "refuse to start, and stop, when free memory is below the memory rule")
	noUnload := set.Bool(flagNoUnload, false, "keep the model loaded after the batch")
	if err := set.Parse(args); err != nil {
		return drainSettings{}, err
	}
	if set.NArg() > 0 {
		return drainSettings{}, fmt.Errorf("unexpected argument %q", set.Arg(0))
	}
	if *module == "" {
		return drainSettings{}, errors.New("--" + flagModule + " names the module to drain")
	}
	if *timeout <= 0 || *maxRequests < 0 {
		return drainSettings{}, errors.New("--" + flagTimeout + " must be positive and --" + flagMaxRequests + " not negative")
	}
	settings := drainSettings{module: *module, timeout: *timeout, maxRequests: *maxRequests, root: *root, memGuard: *memGuard, unload: !*noUnload}
	if settings.root == "" {
		settings.root = e.WorkDir
	}
	configured, err := projectconfig.Load(settings.root)
	if err != nil {
		return drainSettings{}, err
	}
	settings.model = firstNonEmpty(*model, configured.Model)
	parsed, err := endpoint.Parse(firstNonEmpty(*address, configured.Endpoint))
	if err != nil {
		return drainSettings{}, fmt.Errorf("no usable endpoint (--%s or systemone.endpoint): %w", flagEndpoint, err)
	}
	settings.endpoint = parsed
	return settings, nil
}

func (e Env) minFreePercent() int {
	if value, found := e.LookupEnv(memguard.MinFreeVariable); found {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			return parsed
		}
	}
	return memguard.DefaultMinFreePercent
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func presentReport(settings drainSettings, report drain.Report) drainReport {
	out := drainReport{
		Module: settings.module, EndpointDigest: settings.endpoint.Digest(), Model: settings.model,
		Answered: report.Answered(), Unanswered: report.Unanswered(), Remaining: report.Remaining,
		Items:  make([]reportItem, len(report.Items)),
		Unload: reportUnload{Attempted: report.Unload.Attempted, Done: report.Unload.Done, Detail: report.Unload.Detail},
	}
	for index, item := range report.Items {
		out.Items[index] = reportItem{Ticket: item.Ticket, Status: string(item.Status), Source: item.Source, Kind: string(item.Kind), Detail: item.Detail}
	}
	return out
}
