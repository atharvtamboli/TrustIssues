# FatBot change log

## Integrated employer hiring

Verification and hiring share the employer portal header, navigation, theme and mobile styles. Openings, posting and applicant reviews have bookmarkable views. Employers can check individual evidence against the ledger inside the applicant review and send interview instructions. The former employer career URL redirects to the Hiring tab. The separate career page, styles and controller were removed.

## Credential-first portals

Students now land in their credential wallet. Issuers land in the institution review queue. Verifiers land on reference, file and presentation checks, with hiring available through the Hiring tab. All three portals use slate accents, glass panels, responsive controls and a shared theme preference.

The browser walkthrough exercised role-specific login redirects, employer posting, student application, interview invitation, uploaded evidence, issuer approval, QR sharing, verifier ID and file checks, issuer revocation and the resulting revoked status. The walkthrough also checked the phone-width issuer, verifier and student layouts. The local Go suite passed. Live Fabric and physical-camera scanning require their respective environments.

## Implementation history

The following entries preserve the previous phase reports in implementation order. Their descriptions of earlier landing pages and planned work refer to their respective phases.


# Phase 1 execution log

## 2026-11-21 — Baseline

The operator confirmed upload, issue and verify work. Inspected the current project and roadmap. The available router still trusts browser role headers, stores approval state in two files and has no Go test files. Phase 1 will use isolated test data, preserve the original snapshots and record implementation and verification here.

### Storage and compatibility

Replaced independent live JSON writes with a single-writer, encrypted copy-on-write transaction store. Each commit flushes a temporary file and replaces the snapshot before publishing it in memory. Added native writer locks for Windows and Unix. The old chain is decrypted and its links checked on first initialization. Jobs and unassigned pending metadata are imported, and legacy files remain available as evidence. Historical credentials have no inferred owner. A bbolt dependency download failed because the Go module proxy was unreachable; this small local store uses existing dependencies and rewrites one snapshot per transaction.

### Sessions, ownership and issuance

Added operator-provisioned bcrypt accounts and eight-hour HTTP-only sessions. Authentication resolves the role and institution on the server. Added same-origin checks, logout and TLS configuration. The portals now call their own origin. Uploads select an issuing institution and store encrypted evidence under a random ID. The issuer queue, document download, holder history and skill matching enforce ownership. Issuance records the issuer and a durable intent before the local ledger commits the credential. An uncertain write is retryable with the same ID. Escaped user-entered values displayed in portal HTML. Historical lookups are explicitly marked legacy. Revocation is reserved at the ledger interface for Phase 3.

### Running Phase 1 on Windows

Run these from the project folder with the server stopped. Each account command prompts for a password. The organization name on the issuer account becomes available in the holder upload form.

```powershell
.\Add-FatBotUser.ps1 -Username student -Role holder
.\Add-FatBotUser.ps1 -Username dean -Role issuer -Organization "Demo University"
.\Add-FatBotUser.ps1 -Username employer -Role verifier
go run .
```

Open http://127.0.0.1:5500/login. Sign in as the holder to submit, as the dean to review and issue, and as the employer to verify. Sign out between roles or use separate browser profiles. Existing role headers and old direct upload URLs no longer authorize access. Accounts must be provisioned by the operator. The account tool reads the password from standard input and never needs it in a command-line argument.

The local data directory defaults to .fatbot and can be selected with FATBOT_DATA. Back up the whole directory including state.key, state.enc and objects. Restrict its operating-system permissions to the service account. Use FATBOT_ADDR, FATBOT_TLS_CERT and FATBOT_TLS_KEY for a network deployment with direct HTTPS. Sessions expire after eight hours and all sessions end on server restart. This stage runs one writer per data directory. The peer network is Phase 2.

### Files changed

| Files | Responsibility |
|---|---|
| main.go, go.mod | CLI and server startup, account provisioning entry point, legacy-compatible and identity-bound block hashes, deterministic skill ordering, dependency declarations |
| auth.go, login.html, Add-FatBotUser.ps1 | Account creation with a hidden password prompt, authentication, protected role routes, origin checks and sign-out |
| store.go, store_windows.go, store_unix.go | Import, encryption, writer exclusion, atomic snapshot publication and restart handling |
| ledger.go, server.go | Issue/read/revoke interface, local issuance adapter, stable submission and credential IDs, encrypted evidence, scoped queries and jobs |
| holder.html, issuer.html, verifier.html, portal.js | Same-origin requests, institution selector, scoped navigation, authenticated file downloads, retry status and issuer display |
| phase1_test.go | HTTP ownership, approval concurrency, storage-failure, restart and session regression tests |

### Execution results

- Formatted the Go files, compiled the application with go build -mod=readonly and ran go vet ./... successfully.
- go test -mod=readonly -count=1 ./... passed. Six concurrent approvals produced one committed record. A simulated snapshot write failure returned 503 and preserved the pending submission. Another simulated interruption preserved the approval intent through a restart and a successful retry.
- Tested two holders, two issuing institutions and a verifier. History, skill results, queue and evidence checks were scoped to the expected identities. Logged-out, wrong-role and cross-origin requests were rejected.
- Ran the compiled server on an isolated loopback port, provisioned three synthetic accounts through the CLI and signed in and out over actual HTTP for each portal. The holder received the provisioned issuer in the institution selector API. Parsed all portal JavaScript with Node. Browser interactions were not automated.
- Attempted go test -race. The installed Windows MinGW compiler failed to launch during runtime/cgo compilation with an illegal DLL relocation. The normal concurrent HTTP test passed; race-detector coverage remains outstanding.
- All execution used isolated application data. The operator's existing chain, jobs, pending records and uploads were not rewritten. The first operator run imports the legacy snapshot. Unowned legacy approvals remain available by hash and outside personal dashboards. Historical pending metadata is archived for manual identification or re-submission.

The local adapter implements issuance and verification. Its revoke method returns an explicit unsupported error until Phase 3. Phase 2 can now introduce the Fabric network behind the ledger interface. The Phase 1 snapshot store targets the single-node pilot and has not been load-tested.

---

# Phase 2 execution log

## 2026-11-21 — Network bring-up

Read the Phase 2 acceptance criteria and inspected the project and available runtimes. Started the Phase 1 regression suite. The Windows WSL client returned Wsl/E_ACCESSDENIED when asked to list distributions. Docker and Podman were absent from the command path. Network configuration and acceptance tooling can proceed in the project independently of the live runtime.

Phase 1 regressions passed. Added pinned version metadata and generated configurations for two peer organizations, a single Raft orderer, TLS membership identities, both anchor peers, separate ledger volumes and loopback endpoints. Added a WSL/Linux launcher for binary checksum verification, sample checkout, channel creation and chaincode lifecycle. Added a proof command that obtains valid transaction IDs, queries each peer directly, checks both discovery views, compares heights and hashes, and creates a second transaction while the partner is stopped. The launcher restarts the partner in a finally block. Each runtime directory receives its own Compose project and volumes.

Verification: the five offline network tests passed. They cover generated topology and volume separation, direct reads from two distinct organizations, rejection of divergent tip hashes, missing discovery members and restart after an interrupted outage test. The Windows preflight correctly exited nonzero. No Docker engine pipe or client was available, WSL distribution enumeration was denied, and git ls-remote could not resolve github.com. A live channel, sample transaction ID and post-restart block comparison could not be obtained on this host. Phase 2 acceptance remains open pending a successful prove run. The new runbook lists the launch commands, trust roots, port mapping and proof location. No credential data was transferred or migrated.

---

# Phases 3 and 4 execution log

## 2026-11-21

Read the current roadmap, Phase 1 ledger and portal routes, and Phase 2 deployment code. Work begins with a deterministic credential contract, a separate Fabric Gateway signer, and a local implementation for offline operation. Existing historical records retain their original source of trust. Runtime dependencies and live acceptance results are recorded below.

Contract and portal implementation: added v1 credential keys, MSP and enrollment-attribute checks, issuer-only revocation and ledger events. Implemented a versioned salted commitment over document digest and approved private claims, issuer-side submit reconciliation, partner-side status reads, off-chain share links with expiry, local QR generation and a camera scanner. Embedded portal assets under web/ so the compiled server serves the same routes from any working directory. Original data files stayed in place.

## Files and decisions

| Files | Changes |
|---|---|
| chaincode/credential | Fabric contract entry points, deterministic registry and permission/lifecycle tests. Credential records contain the commitment, issuer MSP, status and transaction references. |
| ledger.go, fabric.go, network/gateway | Local history and append-only revocation receipts, separate Go Gateway process, issuer signing selection and partner-targeted reads. Reconciliation persists an endorsed envelope and transaction ID before sending it. |
| server.go, store.go, credentials.go, main.go | Backward-compatible state fields, local/Fabric selection, active-only skills, status and document checks, protected evidence and expiring share endpoints. |
| web/, web_assets.go | Portal files moved together and embedded into the binary. Added issuance references, revocation controls, holder downloads, QR generation and share presentations. The QR encoder is bundled locally with its license. |
| network/phase2.py, network/phases34.py, network/enroll | Reused the lifecycle routine for separate sample and credential contracts; added staged contract/Gateway builds, institute-specific signing profiles, demo certificate enrollment and a live acceptance command. |
| phase34_test.go, network/CREDENTIALS.md | Local workflow and retry tests, conditional live integration suite, commitment vector, operator commands and outstanding environment requirements. |

The pilot policy allows authorized issuers in Org1MSP and Org2MSP to issue. Both organizations endorse credential writes. The pilot generates attributed client certificates from the local development CA. Independent institutional key custody and governance belong to the pilot deployment. A native OpenSSL executable failed to start during development, so enrollment was implemented and verified using Go's standard X.509 library. No separate OpenSSL installation is needed.

## Final functionality checks

- The main application built on Windows and go vet passed. Seven application tests passed, including Phase 1 regressions. The real two-peer test was explicitly skipped because no Fabric configuration was available.
- The registry lifecycle test and the enrollment certificate test passed. The tests checked unauthorized issue, duplicate IDs, cross-MSP revoke, repeated revoke, preserved issuance history, events and the signed issuer attribute.
- Five offline network tests passed. Every portal script parsed successfully.
- A running server with isolated synthetic data accepted a holder upload, issuer approval, document comparison and revocation. The original file matched; a one-byte change returned altered. After revocation the same link reported revoked, denied evidence access and removed the unsupported skill. Separate regression tests checked expired links, wrong holders, duplicate skill support and isolation.
- Headless Microsoft Edge opened the authenticated holder portal and clicked Share. OpenCV independently decoded the QR image drawn on its canvas to the exact URL shown by the portal. Edge opened that presentation and rendered Active credential. A physical phone camera was not available.
- Adapter tests simulated a lost commit response and an interruption before submission. Recovery either reconciled the existing commit or checked transaction status and reused the saved envelope. Partner failure returned unavailable. These are simulated Gateway results.
- The external Go module proxy could not be resolved, including after retrying with a writable dependency cache. The Fabric contract wrapper and Gateway executable therefore remain uncompiled here. Live endorsement, ordering, partner read and revocation transaction IDs remain unverified. The local app, contract transition core and enrollment helper compiled and ran.
- Runtime smoke checks used disposable accounts, ports and data outside the project. The cumulative source archive excludes uploaded documents, private keys, passwords, local ledgers and runtime proofs.

The later WSL2/Docker prerequisites and exact deploy/prove commands are in network/CREDENTIALS.md. No network software was installed and no external organization was contacted.

---

# Career workspace changes

## 1. Company openings
Extended the existing Go job and store types with a stable opening ID, posting account, company and interview instructions. The server assigns ownership, trims duplicate requirements and keeps older openings available for practice. Only a new, owned opening accepts applications.

## 2. Student preparation
Students select a company and role, view matching credential-backed skills, follow course links and save practice progress by opening. Known skills have specific learning resources. Other skills open a course search. Marking a task practiced records personal progress. Verification requires an active credential.

## 3. Career coach
The local guide uses the selected role, interview instructions and skill gaps. To enable generated replies, configure FATBOT_AI_KEY and FATBOT_AI_MODEL in the server environment before starting the app. FATBOT_AI_URL optionally selects a compatible chat completions endpoint. Only submitting a question invokes the provider. Requests include the job specification, learning steps and question. Responses have a token cap and each account has a 15-second cooldown. Provider errors fall back to the local guide.

## 4. Applications and interviews
An application is saved once per student and opening. The posting employer can review verified requirements, see which requirements lack evidence, inspect matching credential references and send interview instructions. Students can read the status and invitation inside the app. Skills from revoked credentials drop out of the match.

## 5. Interface and compatibility
Added an embedded responsive career dashboard with role search, preparation columns, a coach form and an employer review screen. Students and employers land here after login. The credential portal link opens their original tools. Issuers retain their original workflow. Existing Go, Gin and plain JavaScript handle the full workflow.

## 6. Running and checking
From the project folder run `go run .` and open http://127.0.0.1:5500. Use existing holder and verifier accounts to try each side. The local setup works without Docker or WSL.

The Go regression suite covers posting, duplicate application attempts, practice versus verified skills, user and employer isolation, an invitation, credential revocation, local coaching and the AI adapter against a simulated provider. The browser smoke used disposable accounts to publish a role, save practice, apply, ask the guide, invite and read the invitation. Build, Go vet and JavaScript syntax checks were also run. Live model billing and a deployed Fabric network were outside the local test.

---

# Student portal integration

## Navigation and design
Holder sign-in opens the credential portal at /holder. The previous student /career URL redirects into its Openings view. The dashboard, wallet, jobs, eligibility, roadmap, courses, coach, Unstop opportunities and skill assessment use distinct URL fragments, so refresh, back and direct links work. The layout uses the supplied glass reference's colors, surfaces, pill navigation, progress ring and staged cards. It supports a saved dark theme, keyboard focus, reduced motion and narrow screens.

## Working actions
- The wallet uploads evidence to the selected institution, shows pending and issued records, downloads available evidence and opens the existing time-limited QR sharing dialog.
- The job board searches live openings. Choosing a role saves the target for eligibility, the roadmap, learning resources and coaching. The eligibility percentage measures active credential coverage.
- Practice progress and applications use the existing server APIs. Employers' interview invitations appear on the role and job board. Archived roles are available for preparation.
- Upskill & Grow provides skill-specific resources and a starter catalog when the board is empty. Unstop opens external opportunity directories. The four-question fundamentals assessment grades answers and explains each result in the browser.
- The coach uses the existing configured AI adapter or local guide. Its mode is shown with the answer. No external AI account is required for local guidance.

## Verification
The Go regression suite, build, vet and student JavaScript syntax checks ran locally. A headless Edge walkthrough used disposable accounts to post a role, select a target, mark practice, apply, ask the coach and receive an interview invitation. The student uploaded a file through the portal, refreshed after issuer approval, shared a QR code and saw 50% verified coverage for a role with two requirements. The browser also checked the assessment score, Unstop link destinations, course recommendation, theme toggle and phone-width layout of every student view. Third-party registration, paid model calls and a running Fabric network were outside this check.
