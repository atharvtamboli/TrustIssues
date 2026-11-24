# Preview release readiness

## Release scope

Credential and career workflow preview 0.1.0. Named holder, issuer and verifier accounts provide wallet, issuance/revocation, expiring presentation, verification, job applications and skill guidance. New role navigation is documented in the README with screenshots.

- Go 1.24.1+; follow account-provisioning commands in the README before starting `go run .`.
- Run the complete local holder → issuer → verifier journey using disposable credentials and exercise revocation.
- Back up the complete private data directory including the encryption key, and rehearse restore separately.
- Build the portable Windows archive with `python scripts/build.py --os windows --arch amd64`. Release workflow supports other OS/architectures; review all artifacts and checksums before publishing a tag.
- Never distribute local account files, retention data or original uploaded evidence.

## Validation

Reviewed main commit 1a9ee93837c8e06fda12a9daa71da8db96ccd80f. Local Go tests, vet, two skill-planner tests and five Python network tests passed. Navigation PR GitHub Actions passed. Live portal screenshots use disposable local accounts.

## Gates before production

Live independent multi-organization Fabric deployment and acceptance, verified institution enrollment/key custody, consent/privacy and deletion lifecycle, portable credential interoperability, independent review of hosting and security, load testing, real device/ARM64 validation, and signed distribution. Batch issuance and portable standards-based credentials remain roadmap items. The default local ledger is controlled by the installation host.
