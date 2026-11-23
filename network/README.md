# Phase 2 · Fabric development network

Bring up two organizations on one local Docker engine and exercise a cross-peer write, read and recovery. All records in the exercise are synthetic. The credential portals continue using the Phase 1 ledger. Credential chaincode and application integration are scheduled for phases 3 and 4.

## Run on Windows with WSL2

Enable a Linux distribution in WSL2 and enable Linux Docker engine access for that distribution. In its shell, install Git, Go and Python 3.9 or newer. Go must support the toolchain requested by the pinned sample module. The first run needs access to GitHub, the Go module proxy and Docker Hub.

```bash
cd /mnt/c/Users/VICTUS/Desktop/hackathon_blockchain
python3 network/phase2.py doctor
python3 network/phase2.py up
python3 network/phase2.py prove
```

Allow ports 17050, 17051, 17053 and 19051 to be available on loopback. The root directory defaults to `~/.local/share/fatbot-fabric-phase2` inside Linux. Set `FATBOT_FABRIC_HOME` before the first command to select another private directory. Keep the same directory across runs. It contains private keys, binaries, the pinned sample checkout, generated channel configuration and the Compose file. Keep those keys local. Each peer and the orderer have separate persistent Docker volumes. Re-running `up` reuses the installation and chaincode definition.

| Actor | Client endpoint | Advertised peer or orderer address | TLS root relative to the runtime directory |
|---|---|---|---|
| University, Org1MSP | localhost:17051 | peer0.university.fatbot.test:17051 | crypto/peerOrganizations/university.fatbot.test/peers/peer0.university.fatbot.test/tls/ca.crt |
| Partner, Org2MSP | localhost:19051 | peer0.partner.fatbot.test:19051 | crypto/peerOrganizations/partner.fatbot.test/peers/peer0.partner.fatbot.test/tls/ca.crt |
| Single Raft orderer | localhost:17050 | orderer.fatbot.test:7050 | crypto/ordererOrganizations/fatbot.test/orderers/orderer.fatbot.test/tls/ca.crt |

The orderer administration endpoint uses mutual TLS on localhost:17053. Both anchor peers are included in the genesis configuration for channel `credentials`. The development peers use the local Docker socket to build and launch the pinned sample Go chaincode.

## Versions and setup

`versions.json` pins Fabric binaries and the peer, orderer, builder and runtime images to 2.5.16. The setup verifies the release archive against GitHub's published SHA-256 digest, checks out Fabric Samples at commit prefix `5789681b4f4d`, and vendors its Go asset-transfer chaincode. It downloads the Go Gateway dependency at v1.7.1 for subsequent integration. Phase 2 exercises the peers directly using the Fabric CLI. The channel genesis, separate MSPs, certificates, chaincode package and ledger volumes are reused on subsequent runs.

## Acceptance evidence

1. The discovery service on *each* peer must report both organizations with the expected MSP and advertised address.
2. University admin submits a unique sample asset through the university peer. The runner requires its VALID commit event and transaction ID.
3. The runner queries the university and partner peers separately. Both must return the submitted asset and agree on height and current block hash.
4. It records the partner's ledger volume and stops that peer. The university commits a second unique asset, and its height must advance beyond the partner's last recorded height. The sample uses a single-organization endorsement option for this outage exercise. Both organizations participate in the chaincode lifecycle.
5. It restarts the partner, checks the volume identity, and waits for matching asset values, heights and block hashes. A successful run writes a uniquely named `proof-phase2-*.json` in the runtime directory. It contains both transaction IDs, both discovery results, block hashes, heights, volume name and timestamp.

If the acceptance run fails during the outage, it attempts to restart the partner before exiting. Re-run `prove` after correcting the runtime issue. To pause all services while preserving ledgers, run `docker compose -p PROJECT -f "$FATBOT_FABRIC_HOME/compose.yaml" stop` using the generated project name printed by `up` and the actual runtime directory. Run `up` again to resume.

## Offline checks

```bash
python3 -m unittest discover -s network -v
```

These tests check the generated topology and exercise the evidence and recovery logic with fixtures. The live proof file is the acceptance evidence for the network.

[Fabric releases](https://github.com/hyperledger/fabric/releases) · [Sample contract](https://github.com/hyperledger/fabric-samples/tree/5789681b4f4d/asset-transfer-basic/chaincode-go) · [Discovery CLI](https://hyperledger-fabric.readthedocs.io/en/latest/discovery-cli.html)
