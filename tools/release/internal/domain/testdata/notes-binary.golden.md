## discpower 0.2.0

### Changes

- feat(dp): add a thing (abc1234)
- fix(format): mend it (def5678)

Since `discpower/v0.1.0`.

### Install

```
go install github.com/RachidChabane/spec-proof/discpower/cmd/spec-proof-dp@v0.2.0
```

Or download `install` from this release and run `bash install discpower 0.2.0`: it checks the signature, the checksum and the version before it installs.

### Verify

Download `SHA256SUMS`, `SHA256SUMS.sigstore.json` and an archive, then:

```
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-identity https://github.com/RachidChabane/spec-proof/.github/workflows/release.yml@refs/tags/discpower/v0.2.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
gh attestation verify <archive> --repo RachidChabane/spec-proof
```
