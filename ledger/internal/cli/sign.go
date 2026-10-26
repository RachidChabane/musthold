package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/RachidChabane/musthold/ledger/internal/adapters/digestmanifest"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/filetree"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/schemacheck"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/signing/ed25519signer"
	"github.com/RachidChabane/musthold/ledger/internal/adapters/signing/sigstore"
	"github.com/RachidChabane/musthold/ledger/internal/app"
)

// Identities `sign` can use.
const (
	identityEd25519  = "ed25519"
	identitySigstore = "sigstore"
)

func runSign(environment Environment, args []string, stdout, _ io.Writer) error {
	flags := newFlagSet(environment, signCommand)
	identity := flags.String("identity", "", "signing identity: "+identityEd25519+" or "+identitySigstore)
	in := flags.String("in", "", "directory of statements: <module>/<subject key>/<cacheKey>.json")
	out := flags.String("out", "", "directory the signed bundles are written to")
	digests := flags.String("digests", "", "digest manifest (sha256sum format) of the statements")
	keyFile := flags.String("key-file", "", "PKCS#8 PEM Ed25519 key file readable by its owner only")
	fulcioURL := flags.String("fulcio-url", sigstore.PublicFulcioURL, "Fulcio certificate authority")
	rekorURL := flags.String("rekor-url", sigstore.PublicRekorURL, "Rekor transparency log")
	if err := parse(flags, args); err != nil {
		return err
	}
	for name, value := range map[string]string{"identity": *identity, "in": *in, "out": *out} {
		if err := requireFlag(flags, name, value); err != nil {
			return err
		}
	}
	signer, err := newSigner(environment, *identity, *keyFile, *fulcioURL, *rekorURL)
	if err != nil {
		return err
	}
	schemas, err := schemacheck.New()
	if err != nil {
		return fmt.Errorf("load schemas: %w", err)
	}
	var integrity app.IntegrityCheck = app.NoIntegrity{}
	if *digests != "" {
		integrity = digestmanifest.File{Path: *digests}
	}
	service := app.SignService{
		Source:    filetree.StatementDir{Root: *in},
		Integrity: integrity,
		Schemas:   schemas,
		Signer:    signer,
		Sink:      filetree.BundleDir{Root: *out},
	}
	report, err := service.Run()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "signed %d statements with %s\n", report.Signed, *identity)
	return nil
}

func newSigner(environment Environment, identity, keyFile, fulcioURL, rekorURL string) (app.Signer, error) {
	switch identity {
	case identityEd25519:
		key, err := ed25519signer.KeySource{File: keyFile, LookupEnv: environment.LookupEnv}.Load()
		if err != nil {
			return nil, err
		}
		return ed25519signer.New(key)
	case identitySigstore:
		token, err := sigstore.TokenSource{LookupEnv: environment.LookupEnv}.Token(context.Background())
		if err != nil {
			return nil, err
		}
		return sigstore.NewKeyless(sigstore.KeylessConfig{FulcioURL: fulcioURL, RekorURL: rekorURL, IDToken: token})
	default:
		return nil, fmt.Errorf("%w: sign: --identity must be %s or %s", errUsage, identityEd25519, identitySigstore)
	}
}
