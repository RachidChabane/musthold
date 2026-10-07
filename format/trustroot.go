package format

import _ "embed" // the TUF root is embedded as a byte slice

//go:embed tuf/public-good-root.json
var publicGoodTUFRoot []byte

// PublicGoodTUFRoot returns the TUF root of the public-good Sigstore instance. It
// is the root every trusted root is verified back to: the verifier checks the TUF
// metadata of the trusted root against it, and the ledger fetches the snapshot of
// that metadata from the root's version on. The bytes are a copy.
func PublicGoodTUFRoot() []byte {
	return append([]byte(nil), publicGoodTUFRoot...)
}
