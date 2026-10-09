# Security Policy

We take the security of Nightingale seriously. Thank you for helping keep Nightingale and its users safe.

## Supported Versions

Security fixes are released for the latest minor version of the current major release.

| Version | Supported          |
| ------- | ------------------ |
| 9.1.x   | :white_check_mark: |
| < 9.1   | :x:                |

If you are running an older version, please upgrade to the latest release before reporting an issue. Make sure the issue still reproduces there.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

Report vulnerabilities privately through GitHub's private vulnerability reporting:

1. Go to the [Security tab](https://github.com/ccfos/nightingale/security) of this repository.
2. Click **Report a vulnerability**.
3. Fill in the form. Only the maintainers and you can see the report.

Please include as much of the following as you can:

- The affected version(s) and deployment method (binary, Docker, Helm, etc.)
- The affected component (e.g. `center`, `alert`, `pushgw`, API endpoint, UI page)
- Step-by-step instructions to reproduce the issue
- A proof of concept or exploit code, if available
- The impact of the issue, including how an attacker might exploit it

## What to Expect

- **Acknowledgement:** within 3 business days of your report.
- **Initial assessment:** within 10 business days. We will confirm whether the issue is accepted and share an initial severity assessment.
- **Fix and disclosure:** we aim to release a fix within 90 days of the report. Critical issues will be prioritized. We will keep you updated on progress.

Once a fix is available, we will publish a [GitHub Security Advisory](https://github.com/ccfos/nightingale/security/advisories) and request a CVE where appropriate. We will credit you in the advisory unless you prefer to remain anonymous.

## Coordinated Disclosure

We ask that you:

- Give us a reasonable amount of time to fix the issue before any public disclosure.
- Avoid accessing or modifying data that does not belong to you, and avoid degrading the service of others.
- Only test against your own Nightingale deployments.

We will not pursue legal action against researchers who report vulnerabilities in good faith and follow this policy.

## Scope

In scope:

- The Nightingale server and its components in this repository
- Official release binaries and Docker images built from this repository

Out of scope:

- Vulnerabilities in third-party dependencies that are already publicly known and do not affect Nightingale in practice. Please still let us know if you find one that is exploitable through Nightingale.
- Issues that require an already-compromised host or administrator account
- Insecure configurations explicitly documented as unsafe for production (e.g. default credentials not changed after installation)
- Denial of service through high-volume traffic

Vulnerabilities in [Categraf](https://github.com/flashcatcloud/categraf) should be reported to that repository.
