# Security Policy

We take the security of Nightingale seriously. Thank you for helping keep Nightingale and its users safe.

## Supported Versions

The current major version (v9) is supported. Security fixes are shipped in the latest v9 release and are not backported to earlier minor or patch releases.

| Version | Supported          |
| ------- | ------------------ |
| 9.x     | :white_check_mark: |
| < 9.0   | :x:                |

If you are running an older release, please upgrade to the latest release before reporting an issue. Make sure the issue still reproduces there.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

Report vulnerabilities privately through GitHub's private vulnerability reporting:

1. Go to the [Security tab](https://github.com/ccfos/nightingale/security) of this repository.
2. Click **Report a vulnerability**.
3. Fill in the form. Only the maintainers and you can see the report.

Complete the required fields in the [private reporting form](.github/VULNERABILITY_REPORT.yml). Reports must include:

- The exact tested release or commit, version output, and binary/image/build provenance. Verify the issue on the latest supported release or current `main` and explain any testing limitations.
- The affected component and entry point, deployment method, relevant effective configuration, and all changes from the shipped configuration.
- The attacker's authentication, exact granted permissions, resource ownership/group membership, and the intended security boundary the action exceeds.
- Complete setup and reproduction steps, a minimal runnable proof of concept, and sanitized requests, responses, or equivalent output from an actual run, with a screenshot showing the attack input and observed unauthorized result.
- The expected restriction and an observed normal or denied control comparison on the same tested instance/version, with a separate screenshot showing the actual input and result and an explanation of the identity, permissions, and configuration used. Terminal screenshots are acceptable for API or CLI results. If a safe control cannot be run, explain the limitation and provide independent runtime verification of the boundary with a separate screenshot.
- The demonstrated impact, necessary deployment/upstream conditions, related public reports or fixes checked, and a clear list of unverified assumptions.
- Whether AI tools assisted the report; if so, name the tools, describe their contribution, and state which reproduction and evidence you personally ran and checked.

Submit one root cause per report, including its related affected endpoints. Keep essential steps and results inline. Upload at least two distinct screenshots directly through the private report: the actual attack result in the attack-results field and the control result in the control-results field. Reusing the same screenshot twice counts as one. External image hosts, repository-committed images, relative image paths, and badges are not accepted as screenshot evidence. Caption each image with its reproduction step, tested version, test time, and identity. Other attachments may supplement this evidence. Use synthetic data and redact real credentials and personal or third-party data.

Personally reproduce the issue on a running Nightingale instance and identify mocked upstreams, modified product code, or omitted middleware. Code inspection, scanner output, unit tests, and handler-only harnesses may supplement the instance reproduction but do not replace it. AI-assisted reports are welcome; the reporter remains responsible for verifying the observations. `Pending`, `TODO`, unmodified prompts, and promises to attach evidence later do not satisfy required fields. Reports with missing evidence, hypothetical results presented as observations, or no demonstrated security boundary may be returned for more information or closed.

Administrative credentials and explicitly granted configuration or user-management permissions carry their intended administrative capabilities. A report involving those capabilities must explain an action outside their delegated scope; possession of the permission alone is not an authorization bypass.

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
