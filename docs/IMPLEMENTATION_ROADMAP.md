# FatBot credential network implementation roadmap

**First release:** A university approves a credential in the existing Go application. An employer checks the original document through a peer operated by a second organization. When the university revokes the credential, the employer sees the updated status. Demonstrate this with separate ledger volumes and independently administered identities.

## Where the project stands

| Area | What the project contains | What remains |
|---|---|---|
| Credential workflow | Three HTML portals and a Gin API. Holders upload files and submit skill tags. An issuer reviews pending requests and approves by file hash. | Bind each request to a holder and an issuing institution. Give the issuer a way to approve the actual claims and revoke the result. |
| Go ledger | `createBlock` links each approval to the previous block. SHA-256 covers index, timestamp, filename, file hash, previous hash and skills. The server persists the slice to an AES-GCM file. | One process controls history. Startup decrypts and unmarshals without validating links or recomputing hashes. The block hash omits status and role. There are no transaction signatures, peers, consensus or deployed contracts. |
| Verification | The browser can hash a selected file locally. The API accepts either a block hash or file hash and labels any match verified. The QR control opens a prompt. | Distinguish current status, issuer authority, document match and service availability. Implement a real share-and-scan flow. |
| Identity and privacy | Handlers trust the caller-supplied Role header. The chain stores filenames and skill tags. The all-files endpoint and dashboard read global records. | Authenticate users, authorize per object and organization, and move personal information off the shared ledger. |
| Reliability | The server keeps three global slices and writes separate files. Approval removes the pending item before saving the new block. Write errors can still produce HTTP success. Failed chain loading can lead to creation of a fresh genesis. | Prevent misleading success, lost requests and accidental replacement of damaged data. The encryption helper hardcodes its key and reuses a zero nonce. |
| Workforce features | Job posting and deterministic skill-overlap scoring exist. All students currently share one computed skill set. | Calculate matches from the signed-in holder's active, issuer-confirmed skills. Evaluate ML after this baseline works. |

**Data inspected:** The supplied snapshot contains four blocks including genesis. All three approval records and genesis match the current hash formula and all previous-hash links match. The folder contains two jobs, zero pending entries and no uploaded files. File contents and historical issuer identities therefore cannot be verified from this snapshot.

**Build check:** `go test -mod=readonly ./...` compiled the package and reported no test files. Portal behavior was reviewed from the source. No live browser flow or multi-node deployment was exercised in this review.

## Architecture decision

The README selects Hyperledger Fabric and Raft. The older PLAN.md instead outlines a custom Go protocol with libp2p, rotating signers, fork selection and a contract VM. Adopt the README's Fabric design for the credential product. This preserves the Go API, portals, hashing and matching work, and moves ledger replication, ordering and contract lifecycle to Fabric. The learning goal of implementing an original protocol would be a separate project with a different consensus design.

| Participant | Responsibility |
|---|---|
| University organization | Operates its peer, enrolls authorized issuer identities, reviews evidence and signs issuance or revocation. |
| Independent partner organization | Operates a second peer and its own gateway access. It can verify from its own ledger and participate in endorsement under the agreed policy. Endorsing correct contract execution is distinct from attesting that a degree was earned. |
| Ordering service | Sequences endorsed transactions into blocks. Start with the local sample. For a crash-tolerant pilot, deploy three Raft orderers across independent failure domains. Two must remain available to order new writes. |
| Holders and employers | Use the portals as clients. Holders share an individual credential. Employers compare the supplied document with a current credential record. An employer can use a partner gateway without operating a peer. |
| Go service and private storage | Gin handles login, routing and file access. A transactional database holds submissions, identity mappings, private claims and jobs. Private object storage holds the actual files. |

**Data boundary:** Put an opaque credential ID, issuer organization, versioned commitment and revocation state on the channel. Bind the commitment to the exact file digest and issuer-approved claims using a specified serialization and a per-credential random salt. Generate the salt once outside chaincode. Share the evidence and salt only with an authorized verifier. Keep filenames, raw documents, names, skills, contact information and share tokens off-chain. A status-only lookup by ID confirms the registry entry. A document check must also compare the presented evidence to its commitment. Proof that the presenter controls the holder identity is a separate check if required.

## Ordered implementation backlog

Complete the checks in order. The named packages and directories are suggested destinations for new work.

### Phase 0 — Capture a reproducible baseline

- [ ] **0.1** Copy the demo state into a private backup. Record checksums, block/link results and counts. Use synthetic documents for development.
- [ ] **0.2** Extract Gin router construction from `main()`. Inject a temporary data directory so tests can never modify the supplied snapshot.
- [ ] **0.3** Add a workflow test covering upload → pending → approve → verify exact bytes → restart → verify again. Include a different file for the unknown result.
- [ ] **0.4** Distinguish missing and corrupt storage at startup. On truncation or failed authentication, stop and preserve the input. If legacy writes continue, use a fresh GCM nonce per write, an external key and a versioned reader for old data.

**Pass when:** A clean checkout or project copy can run the synthetic workflow twice. A corrupted fixture produces a clear error and remains byte-for-byte intact.

### Phase 1 — Establish identity, ownership and reliable submissions

- [ ] **1.1** Add authenticated sessions. Resolve user, organization and roles server-side. Have the three portals call the API on their own origin so a second machine reaches its own service.
- [ ] **1.2** Move pending requests, jobs, claims and ownership into a transactional store. Give each upload an opaque ID, immutable storage key, holder ID and intended issuer organization. Check file-save errors.
- [ ] **1.3** Replace the static upload link with an authenticated document endpoint. Scope issuer queues to the assigned institution and dashboard queries to the signed-in holder.
- [ ] **1.4** Introduce a small Go ledger interface for issue, read and revoke behind the routes. Record approval intent durably and assign a stable credential ID before submitting. Represent pending, committed and failed/unknown outcomes explicitly.

**Pass when:** Two holders see different records. Only the assigned issuer can open the document. Concurrent approval requests for one submission create one durable intent. A storage error never returns a completed approval.

### Phase 2 — Bring up two organizations and prove peer connectivity

- [ ] **2.1** Pin compatible Fabric samples, binaries, images and Go Gateway dependencies. Set up the local Fabric test network through WSL2 and a container runtime on Windows. Record exact versions and startup commands under `network/`.
- [ ] **2.2** Create a credential channel. Join one peer per organization with separate MSPs (membership identities), TLS credentials and persistent ledger volumes. Record each organization's endpoint and trust roots.
- [ ] **2.3** Configure advertised endpoints and anchor peers for cross-organization discovery. Verify each peer discovers the other. Use the sample contract for this infrastructure check.
- [ ] **2.4** Submit a sample write through organization A. Query organization B's own peer after commit. Compare channel height and the hash of the same committed block. Restart B with its volume intact and verify it catches up.

**Pass when:** Both peers discover each other and return the same committed value and block hash. Include the peer endpoints and transaction ID in the demonstration so two browser tabs hitting one server cannot masquerade as two peers.

### Phase 3 — Implement the credential smart contract in Go

- [ ] **3.1** Define versioned ledger keys and commitment test vectors. Specify which MSPs may issue and which enrollment attribute identifies an issuer. Agree the endorsement policy with both organizations.
- [ ] **3.2** Implement `IssueCredential` in `chaincode/credential/`. Derive issuer identity from the Fabric transaction. Check issuer authorization and ID uniqueness. Store the approved commitment and emit an issuance event. Use deterministic inputs.
- [ ] **3.3** Implement `ReadCredential` and `RevokeCredential`. Return issuer, commitment and current status. Allow revocation by an authorized identity of the original issuing organization. Preserve issuance history and emit a revocation event.
- [ ] **3.4** Package and install chaincode, approve its definition through both organizations and commit the definition to the channel. Run integration tests for duplicate ID, unauthorized issue, cross-issuer revocation and repeated revocation.

**Pass when:** An issuer creates a credential on A. B reads active status. An unauthorized caller cannot revoke it. The original issuer revokes it and B reads revoked status. Record the issue and revoke transaction IDs.

### Phase 4 — Connect the existing product to the ledger

- [ ] **4.1** Implement the Go Gateway adapter. Bind each authenticated issuer action to an authorized signing identity under that institution's control. Keep signing keys outside HTML and source control.
- [ ] **4.2** Route `/approve` through the adapter. Save the Fabric transaction ID and wait for a valid commit before marking a submission issued. On a timeout, reconcile commit status and the stable credential ID before retrying. Retain the submission for recovery.
- [ ] **4.3** Add a verifier flow using the partner's peer. Return distinct results for active and matching, revoked, unknown, altered document and ledger unavailable. Show the issuer, checked time and transaction reference where applicable.
- [ ] **4.4** Add issuer revocation controls and holder-generated expiring QR share links. Test with a real phone camera. Gate access to the off-chain evidence and support link expiry independently of credential revocation.
- [ ] **4.5** Update the matching input to use only the current holder's active credentials. Drop revoked skill evidence and retain a skill if another active credential still supports it.

**Pass when:** Issue on A, scan and verify on B, change one byte and see a mismatch, revoke on A and see revoked on B. A repeated approval after a simulated timeout yields one credential. A second holder's dashboard never acquires the first holder's skills.

### Phase 5 — Move from a laptop demo to an independently operated pilot

- [ ] **5.1** Give the partner its own host, administrator, peer storage and signing credentials. Replace sample network material. Agree who can add organizations, change endorsement policy, approve upgrades and rotate compromised issuer identities.
- [ ] **5.2** Run three Raft orderers across failure domains. Write with one stopped. Then remove quorum, confirm there are no newly committed writes, restore quorum and check convergence. Separately test loss of any peer required for endorsement.
- [ ] **5.3** Back up and restore peer/configuration material, private objects, database records and necessary keys. Add monitoring for peer height, delayed commits, failed reconciliation and verifier latency. Define when a lagging peer must return status unavailable.
- [ ] **5.4** Dry-run legacy migration. Archive all three legacy approvals with provenance. Request original evidence and fresh issuer attestation before publishing a new credential. Reconcile counts, including entries awaiting evidence. Preserve rollback to the read-only archive.
- [ ] **5.5** Benchmark the README's sub-two-second verification target. Define the proposed acceptance metric as p95 time from verifier submission to a displayed result. Record concurrency, dataset size, peer lag and error rate. Measure issuance-to-visibility separately.

**Pass when:** Independent operators can reproduce the end-to-end demo, recover from the tested failures and show measured verification latency. Report dataset size and load alongside the result. Larger scale remains a target until tested.

### Phase 6 — Extend the workforce product

- [ ] **6.1** Add a portable signed credential format and holder presentation flow. Test an exported credential with an independent implementation and check its current registry status.
- [ ] **6.2** Add batch issuance with per-row outcomes and stable IDs. Replaying a batch must preserve every previously committed credential.
- [ ] **6.3** Measure the current skills-overlap matcher on representative, consented examples. Use that baseline to evaluate recommendations and skill-gap suggestions.

**Pass when:** A holder can share a portable credential, an issuer can retry a partial batch safely, and matching results are measurable against the current algorithm.

**Start with 0.1–0.3.** The first engineering deliverable is one repeatable, isolated Go test for the workflow you already have. The first distributed milestone is 2.4. The first product demonstration worth showing an institution is the Phase 4 pass check.

## Technical references

- [Fabric test network](https://hyperledger-fabric.readthedocs.io/en/latest/test_network.html)
- [Fabric peers](https://hyperledger-fabric.readthedocs.io/en/latest/peers/peers.html) and [peer discovery and gossip](https://hyperledger-fabric.readthedocs.io/en/latest/gossip.html)
- [Fabric ordering and Raft quorum](https://hyperledger-fabric.readthedocs.io/en/latest/orderer/ordering_service.html)
- [Fabric Gateway and commit status](https://hyperledger-fabric.readthedocs.io/en/latest/gateway.html)
- [Go AEAD nonce requirements](https://pkg.go.dev/crypto/cipher#AEAD)
