# TrustIssues 0.1.0

First packaged preview of the credential and career workspace.

- Student wallet, issuer review and employer hiring portals.
- Credential issuance, verification, revocation and expiring presentations with QR codes.
- Skill planner with single-skill and two-skill recommendations and study brief export.
- Encrypted local storage and an optional Hyperledger Fabric gateway.
- Portable binaries for Windows, Linux and macOS on AMD64 and ARM64.
- Windows per-user installer and Start menu launcher.

Windows is the local validation platform. Linux is checked in CI. macOS and ARM64 archives are cross-compiled and need device validation.

The default backend uses a local hash-linked ledger. A configured Fabric network is required for distributed ledger operation. Accounts must be provisioned before sign-in. Back up the complete data directory and its key together.
