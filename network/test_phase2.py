"""Offline regression checks for the topology and the acceptance runner."""
import base64
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import phase2 as net
from topology import CHANNEL, ORGS, channel_config, compose_config, crypto_config, peer_name


class NetworkChecks(unittest.TestCase):
    def test_membership_storage_and_ports(self):
        crypto = crypto_config()
        profile = channel_config(Path("/private/crypto"))["Profiles"]["FatBot"]
        compose = compose_config(net.VERSIONS["fabric"])
        self.assertEqual(len(crypto["PeerOrgs"]), 2)
        self.assertEqual(profile["Orderer"]["OrdererType"], "etcdraft")
        self.assertEqual(len(compose["volumes"]), 3)
        for i, (org, _, port) in ORGS.items():
            anchor = profile["Application"]["Organizations"][i-1]
            self.assertEqual(anchor["ID"], org + "MSP")
            self.assertEqual(anchor["AnchorPeers"], [{"Host": peer_name(i), "Port": port}])
            service = compose["services"][peer_name(i)]
            self.assertIn(f"peer{i}-ledger:/var/hyperledger/production", service["volumes"])
            self.assertEqual(service["environment"]["CORE_PEER_GOSSIP_EXTERNALENDPOINT"], f"{peer_name(i)}:{port}")
        for service in compose["services"].values():
            self.assertTrue(service["image"].endswith(":" + net.VERSIONS["fabric"]))
            self.assertTrue(all(port.startswith("127.0.0.1:") for port in service["ports"]))

    def test_reads_are_independent_and_heads_must_match(self):
        asset = {"ID": "asset1", "Owner": "FatBot", "Color": "blue", "Size": 3, "AppraisedValue": 100}
        with patch.object(net, "peer", return_value=json.dumps(asset)) as query:
            with patch.object(net, "info", side_effect=[{"height": 4, "hash_base64": "a"},
                                                        {"height": 4, "hash_base64": "b"}]):
                with self.assertRaisesRegex(RuntimeError, "hashes differ"):
                    net.same_state("asset1")
            self.assertEqual([c.args[0] for c in query.call_args_list], [1, 2])

    def test_rejects_missing_discovery_member(self):
        with tempfile.TemporaryDirectory() as tmp:
            user = Path(tmp)
            for d in ("signcerts", "keystore"):
                path = user / "msp" / d
                path.mkdir(parents=True)
                (path / "fixture").touch()
            local = [{"MSPID": "Org1MSP", "Endpoint": peer_name(1)+":17051"}]
            with patch.object(net, "identities", return_value=("Org1MSP", "localhost:17051", "ca", user)), patch.object(net, "run", return_value=json.dumps(local)):
                with self.assertRaisesRegex(RuntimeError, "missing Org2"):
                    net.discovery(1)

    def test_chain_info_parser(self):
        digest = base64.b64encode(bytes(range(32))).decode()
        with patch.object(net, "peer", return_value="Blockchain info: " + json.dumps({"height": 5, "currentBlockHash": digest})):
            self.assertEqual(net.info(2), {"height": 5, "hash_base64": digest})

    def test_failed_outage_write_restarts_partner_and_emits_no_proof(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(net, "ROOT", Path(tmp)), patch.object(net, "discovery", return_value=[]), patch.object(net, "write", side_effect=["first", RuntimeError("interrupted")]), patch.object(net, "same_state", return_value={}), patch.object(net, "volume_b", return_value="volume"), patch.object(net, "compose", return_value="") as compose:
            with self.assertRaisesRegex(RuntimeError, "interrupted"):
                net.prove()
            self.assertEqual(compose.call_args.args, ("start", peer_name(2)))
            self.assertEqual(list(Path(tmp).glob("proof-*.json")), [])


if __name__ == "__main__":
    unittest.main()
