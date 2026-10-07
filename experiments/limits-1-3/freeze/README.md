# Freeze timestamp

`prereg-commit.txt` holds the commit the tag `prereg-limits-1-3` points to.
`prereg-commit.tsr` is an RFC 3161 timestamp of that file issued by DigiCert's
timestamp authority (`http://timestamp.digicert.com`), and `prereg-commit.tsq`
is the query it answers. Read it with:

    openssl ts -reply -in prereg-commit.tsr -text

The message imprint is the SHA-256 of `prereg-commit.txt`; compare that file
with `git rev-parse prereg-limits-1-3^{commit}`.
