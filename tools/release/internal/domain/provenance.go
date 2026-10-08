package domain

// OIDCIssuer is the issuer of the identity token the release workflow signs with.
const OIDCIssuer = "https://token.actions.githubusercontent.com"

// CertificateIdentityPrefix is the start of the certificate identity of the release
// workflow, which a tag completes.
const CertificateIdentityPrefix = RepositoryURL + "/" + ReleaseWorkflow + "@refs/tags/"

// CertificateIdentity is the identity the signing certificate of a release carries:
// the release workflow of the repository at the tag.
func CertificateIdentity(tag Tag) string { return CertificateIdentityPrefix + tag.String() }
