# Freeze timestamp

`prereg-commit.txt` holds the commit the tag `prereg-limits-1-3` points to.
`prereg-commit.tsr` is an RFC 3161 timestamp of that file from DigiCert's
timestamp authority. Check it with:

    openssl ts -reply -in prereg-commit.tsr -text
    openssl ts -verify -data prereg-commit.txt -in prereg-commit.tsr -untrusted <(curl -s https://cacerts.digicert.com/DigiCertTrustedG4TimeStampingRSA4096SHA2562025CA1.crt.pem) -CAfile <system CA bundle>

and compare `git rev-parse prereg-limits-1-3^{commit}` with `prereg-commit.txt`.
