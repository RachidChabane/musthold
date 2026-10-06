# ADR timestamps

`0003-commit.txt` holds the commit that adds ADR 0003, whose pilot switch rules
(SW1-SW6) are fixed before any output of the limits 1-3 pilot is read.
`0003-commit.tsr` is an RFC 3161 timestamp of that file issued by
`http://timestamp.digicert.com` (query: `0003-commit.tsq`). Read it with
`openssl ts -reply -in 0003-commit.tsr -text`; its message imprint is the
SHA-256 of `0003-commit.txt`.
