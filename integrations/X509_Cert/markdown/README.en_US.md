# X509_Cert

Certificate probing input. It monitors the remaining validity and the verification
status of TLS/SSL certificates so they never expire unnoticed.

Two probing styles, which can be mixed in one instance:

- Network probing: handshake with the target and read the certificate chain the
  server presents. Good for public HTTPS/SMTPS endpoints.
- Local files: read certificate files on the host (globs supported). Good for
  certificates you maintain yourself on Nginx, gateways and the like.

## Prerequisites

- Categraf must be able to reach the target (network reachable, not blocked by a
  firewall). Set `http_proxy` if the target is only reachable through a proxy.
- When probing local files, the Categraf process needs read permission on them.
- Turn on `insecure_skip_verify` for self-signed certificates, otherwise the
  verification status stays `invalid`.

## Configuration

Categraf's `conf/input.x509_cert/x509_cert.toml`. `targets` is the only required
option:

```toml
[[instances]]
targets = ["tcp://example.org:443", "https://www.baidu.com", "/etc/nginx/ssl/*.pem"]
labels = { env = "prod" }
```

Supported target forms:

| Form | Description |
| --- | --- |
| `tcp://host:port` | Plain handshake with the port, the most generic form |
| `https://host` | HTTPS site, port defaults to 443 |
| `smtp://host:port` | SMTP STARTTLS |
| `udp://host:port` | DTLS |
| `/path/to/cert.pem` | Local certificate file, `*` glob supported |
| `file:///path/to/*.pem` | Relative paths need the `file://` prefix |

Other common options:

- `timeout`: SSL connection timeout, defaults to 5s
- `server_name`: SNI, required when one IP serves certificates for several domains
- `exclude_root_certs`: only report leaf certificates, which cuts the metric volume
- `use_tls` / `tls_cert` / `tls_key`: only needed when the target requires a client
  certificate (mutual TLS)

## Metrics

| Metric | Description |
| --- | --- |
| x509_cert_expiry | Seconds until the certificate expires, negative once expired. This is what you normally alert on |
| x509_cert_enddate | Expiration timestamp in seconds |
| x509_cert_startdate | Not-before timestamp in seconds |
| x509_cert_age | Seconds since the certificate was issued |
| x509_cert_verification_code | Verification result, 0 = passed, 1 = failed (expired, name mismatch, broken chain, ...) |
| x509_cert_ocsp_status_code | OCSP revocation status, 0=good 1=revoked 2=unknown, present only when the target returns an OCSP staple |

Labels attached to the metrics: `target`, `common_name`, `issuer_common_name`,
`verification` (valid/invalid), `type` (leaf/intermediate/root), `serial_number`,
`signature_algorithm`, `public_key_algorithm` and more.

Alert directly on `x509_cert_expiry`, for example when less than 14 days
(1209600 seconds) are left, and on `x509_cert_verification_code == 1` to catch
broken certificate chains.
