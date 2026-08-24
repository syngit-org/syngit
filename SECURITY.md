# Security Policy

## Supported Versions

syngit is pre-1.0 and does not maintain long-lived release branches. Security fixes are released on top of the latest published minor version only. Older minor versions do not receive backports, so please upgrade before reporting an issue that is already fixed upstream.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, pull requests, or discussions.**

Report them privately through GitHub's private vulnerability reporting.

<https://github.com/syngit-org/syngit/security/advisories/new>

If you cannot use that form, email the maintainer at <dassieu.damien@gmail.com> instead.

Please include as much of the following as you can, so the report can be triaged without a round trip:

- the syngit version (or image digest) and the Kubernetes version it runs on;
- the affected component (controller, webhook, a specific CRD, the Helm chart);
- a description of the impact, e.g. privilege escalation, leaking of Git credentials held in a `RemoteUser` secret, or bypass of a `RemoteSyncer` interception rule;
- the steps or manifests needed to reproduce it;
- any suggested mitigation you already know of.

## Disclosure Process

- We aim to acknowledge a report within **5 business days**.
- We aim to confirm the vulnerability and produce a remediation plan within **30 days** of acknowledgement.
- Fixes are shipped in a new release, and a GitHub Security Advisory with a CVE identifier is published once the fix is available.
- We follow coordinated disclosure: please give us a reasonable window to ship a fix before disclosing publicly. Reporters are credited in the advisory unless they ask not to be.

syngit is maintained by volunteers on a best-effort basis. If a deadline above is going to slip, we will say so in the advisory thread rather than let the report go quiet.

## Scope

In scope:

- the syngit controller and its admission/interception webhooks;
- the published container images and the Helm chart in this repository;
- handling of Git provider credentials and of the SOPS/age keys used by the providers.

Out of scope:

- vulnerabilities in upstream Kubernetes, in the Git providers themselves, or in third-party dependencies;
- findings that require an attacker to already hold cluster-admin, or to already be able to read arbitrary secrets in the syngit namespace;
- missing hardening that has no demonstrated impact.
