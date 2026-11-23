# Run the credential network

## Use the upgraded portals now

From the project folder, run `go run .` and sign in at http://127.0.0.1:5500/login. Existing accounts and data use the same directory as Phase 1. For a fresh installation, create accounts with `Add-FatBotUser.ps1` as documented in the Phase 1 entry of [CHANGELOG.md](../CHANGELOG.md). Local issuance, revocation, document comparison and hour-long QR shares work with the local ledger. Verification displays the ledger source.

1. The holder uploads a credential, selects the institution and submits skill claims.
2. The issuer opens the evidence, reviews the skills and approves the submission. The holder's card now shows the credential ID and issuance reference.
3. The holder selects **Share**. A verifier can open the link or scan its QR code. The link gives its bearer one hour of access to the evidence, even without a portal account.
4. The presentation shows current status. Selecting the original file checks the document digest together with the approved claims. Changing a byte produces a mismatch. The issuer can revoke under **Issued credentials**.
5. Reopen the same link. A revoked presentation reports revoked status and denies downloads. The dashboard counts a skill when another active credential still supports it.

For a phone test, use an HTTPS address the phone can reach, sign in to the holder portal using that address and then create the QR code. A code generated on localhost points back to the scanning device. Use a trusted certificate for the host, configure the app's FATBOT_TLS_CERT, FATBOT_TLS_KEY and FATBOT_ADDR, and allow the phone to reach the app port. Use the native camera app or the verifier scanner on browsers with BarcodeDetector support.

## Run the Fabric acceptance checks later

Use a Linux WSL2 distribution with Docker Engine access, Docker Compose v2, Git, Go and Python 3.9+. The first build needs GitHub, Docker Hub and the Go module proxy. The network defaults to four loopback ports, 17050, 17051, 17053 and 19051. The Phase 2 runbook lists the node identities and TLS roots.

```bash
cd /mnt/c/Users/VICTUS/Desktop/hackathon_blockchain
python3 network/phase2.py doctor
python3 network/phase2.py up
python3 network/phase2.py prove
python3 network/phases34.py deploy
python3 network/phases34.py prove
```

Deployment uses the existing channel. It builds and vendors the contract outside the project, builds the pinned Gateway client, deploys chaincode named `credential` at sequence 1, and prepares local pilot signing profiles. Both organizations must endorse credential writes. The sample asset contract keeps its separate policy for the Phase 2 outage exercise.

The live test creates synthetic accounts and a temporary application database. It issues two sample credentials on the channel, verifies through the partner, checks a changed file, revokes one credential and checks skill retention. It also requests endorsement for a duplicate ID, an unauthorized reader's write and a cross-institution revocation. A successful live run prints the issue and revoke transaction IDs. The synthetic channel entries persist.

To run the portals against the network, use the same WSL environment as the generated paths and signer executable. From the project root:

```bash
export FATBOT_FABRIC_CONFIG="${FATBOT_FABRIC_HOME:-$HOME/.local/share/fatbot-fabric-phase2}/credential-service/config.json"
go run .
```

The default organization names for issuer accounts are `Demo University` and `Partner College`. Each maps to a distinct MSP. To use existing accounts with other institution names, edit the corresponding keys in the private configuration while retaining the correct one-to-one MSP mapping. Verification uses the partner reader profile. The pilot provisions certificates using its local cryptogen CA keys. An independently administered deployment needs enrollment and key custody controlled by each institution.

## Ledger and recovery details

The Fabric key is `credential:v1:<64 lowercase hex ID>`. The value holds the ID, issuing MSP, salted commitment, current status and transaction references. Issuing and revoking require an allowed MSP and the enrolled X.509 attribute `fatbot.issuer=true`. A second valid revoke returns the existing state and preserves the first revocation reference.

The v1 commitment is SHA-256 over length-prefixed UTF-8 fields. The fields, in order, are the literal `fatbot/credential/v1`, random salt, credential ID, holder ID, institution name, filename, lowercase SHA-256 file digest and each skill in its approved order. Each length uses four unsigned big-endian bytes. The pinned vector is in `phase34_test.go`. Only the commitment and registry fields reach Fabric. Documents and identifying claims stay in the private app store.

The app saves the endorsed envelope and transaction ID before submission. An uncertain response is retried against the same ID. The issuer checks committed state and outstanding transaction status before reusing the saved envelope. The verifier directs its ledger read to Org2. An unavailable peer produces an unavailable result. Historical local credentials keep their original provenance. A share link expires independently of revocation, and its secret is carried in the URL fragment.

## Offline checks

```bash
go test -count=1 ./...
go vet ./...
(cd chaincode/credential && go test -count=1 ./registry)
python3 -m unittest discover -s network -v
```

The Fabric-facing Go modules have separate dependency manifests. The full build runs during deployment. A successful offline test run does not exercise Fabric endorsement, transaction ordering or the physical phone camera.
