"""Generate the Phase 2 cryptogen, channel and Compose configurations."""

CHANNEL = "credentials"
PROJECT = "fatbot_phase2"
ORDERER = "orderer.fatbot.test"
ORGS = {1: ("Org1", "university.fatbot.test", 17051),
        2: ("Org2", "partner.fatbot.test", 19051)}
ORDER_PORT = 17050
ADMIN_PORT = 17053


def peer_name(i):
    return "peer0." + ORGS[i][1]


def crypto_config():
    return {
        "OrdererOrgs": [{"Name": "Orderer", "Domain": "fatbot.test",
                         "EnableNodeOUs": True,
                         "Specs": [{"Hostname": "orderer", "SANS": ["localhost", "127.0.0.1"]}]}],
        "PeerOrgs": [{"Name": org, "Domain": domain, "EnableNodeOUs": True,
                      "Template": {"Count": 1, "SANS": ["localhost", "127.0.0.1"]},
                      "Users": {"Count": 1}} for org, domain, _ in ORGS.values()]
    }


def implicit(rules):
    return {key: {"Type": "ImplicitMeta", "Rule": rule} for key, rule in rules.items()}


def channel_config(crypto):
    def org_group(name, path, is_orderer=False):
        msp = name + "MSP"
        policies = {}
        for label, role in (("Readers", "member"), ("Writers", "member"),
                            ("Admins", "admin"), ("Endorsement", "peer")):
            if is_orderer and label == "Endorsement":
                continue
            policies[label] = {"Type": "Signature", "Rule": f"OR('{msp}.{role}')"}
        return {"Name": msp, "ID": msp, "MSPDir": str(path / "msp"), "Policies": policies}

    order_dir = crypto / "ordererOrganizations" / "fatbot.test"
    order_org = org_group("Orderer", order_dir, True)
    order_org["OrdererEndpoints"] = [ORDERER + ":7050"]
    tls = str(order_dir / "orderers" / ORDERER / "tls" / "server.crt")
    base = {"Readers": "ANY Readers", "Writers": "ANY Writers", "Admins": "MAJORITY Admins"}
    order = {"OrdererType": "etcdraft", "EtcdRaft": {"Consenters": [
        {"Host": ORDERER, "Port": 7050, "ClientTLSCert": tls, "ServerTLSCert": tls}]},
        "BatchTimeout": "1s", "BatchSize": {"MaxMessageCount": 10,
        "AbsoluteMaxBytes": "10 MB", "PreferredMaxBytes": "512 KB"},
        "Organizations": [order_org], "Capabilities": {"V2_0": True},
        "Policies": implicit(dict(base, BlockValidation="ANY Writers"))}
    peers = []
    for i, (name, domain, port) in ORGS.items():
        org = org_group(name, crypto / "peerOrganizations" / domain)
        org["AnchorPeers"] = [{"Host": peer_name(i), "Port": port}]
        peers.append(org)
    app = {"Organizations": peers, "Capabilities": {"V2_0": True},
           "Policies": implicit(dict(base, Endorsement="MAJORITY Endorsement",
                                     LifecycleEndorsement="MAJORITY Endorsement"))}
    return {"Profiles": {"FatBot": {"Policies": implicit(base), "Capabilities": {"V2_0": True},
                                    "Orderer": order, "Application": app}}}


def compose_config(version, project=PROJECT):
    order_dir = "./crypto/ordererOrganizations/fatbot.test/orderers/" + ORDERER
    order_env = {"ORDERER_GENERAL_LISTENADDRESS": "0.0.0.0", "ORDERER_GENERAL_LISTENPORT": "7050",
                 "ORDERER_GENERAL_LOCALMSPID": "OrdererMSP", "ORDERER_GENERAL_LOCALMSPDIR": "/node/msp",
                 "ORDERER_GENERAL_BOOTSTRAPMETHOD": "none", "ORDERER_CHANNELPARTICIPATION_ENABLED": "true",
                 "ORDERER_GENERAL_TLS_ENABLED": "true", "ORDERER_GENERAL_TLS_CERTIFICATE": "/node/tls/server.crt",
                 "ORDERER_GENERAL_TLS_PRIVATEKEY": "/node/tls/server.key", "ORDERER_GENERAL_TLS_ROOTCAS": "[/node/tls/ca.crt]",
                 "ORDERER_GENERAL_CLUSTER_CLIENTCERTIFICATE": "/node/tls/server.crt",
                 "ORDERER_GENERAL_CLUSTER_CLIENTPRIVATEKEY": "/node/tls/server.key",
                 "ORDERER_GENERAL_CLUSTER_ROOTCAS": "[/node/tls/ca.crt]",
                 "ORDERER_ADMIN_LISTENADDRESS": "0.0.0.0:7053", "ORDERER_ADMIN_TLS_ENABLED": "true",
                 "ORDERER_ADMIN_TLS_CLIENTAUTHREQUIRED": "true", "ORDERER_ADMIN_TLS_CERTIFICATE": "/node/tls/server.crt", "ORDERER_ADMIN_TLS_PRIVATEKEY": "/node/tls/server.key",
                 "ORDERER_ADMIN_TLS_CLIENTROOTCAS": "[/node/tls/ca.crt]"}
    services = {ORDERER: {"image": "hyperledger/fabric-orderer:" + version,
                "environment": order_env, "ports": [f"127.0.0.1:{ORDER_PORT}:7050", f"127.0.0.1:{ADMIN_PORT}:7053"],
                "volumes": [order_dir + ":/node:ro", "order-ledger:/var/hyperledger/production/orderer"]}}
    volumes = {"order-ledger": {}}
    for i, (org, domain, port) in ORGS.items():
        name = peer_name(i)
        endpoint = f"{name}:{port}"
        volume = f"peer{i}-ledger"
        volumes[volume] = {}
        env = {"CORE_PEER_ID": name, "CORE_PEER_ADDRESS": endpoint, "CORE_PEER_LISTENADDRESS": f"0.0.0.0:{port}",
               "CORE_PEER_CHAINCODEADDRESS": f"{name}:{port+1}", "CORE_PEER_CHAINCODELISTENADDRESS": f"0.0.0.0:{port+1}",
               "CORE_PEER_GOSSIP_BOOTSTRAP": endpoint, "CORE_PEER_GOSSIP_EXTERNALENDPOINT": endpoint,
               "CORE_PEER_GOSSIP_ORGLEADER": "true", "CORE_PEER_GOSSIP_USELEADERELECTION": "false",
               "CORE_PEER_LOCALMSPID": org + "MSP", "CORE_PEER_MSPCONFIGPATH": "/node/msp",
               "CORE_PEER_TLS_ENABLED": "true", "CORE_PEER_TLS_CERT_FILE": "/node/tls/server.crt",
               "CORE_PEER_TLS_KEY_FILE": "/node/tls/server.key", "CORE_PEER_TLS_ROOTCERT_FILE": "/node/tls/ca.crt",
               "CORE_VM_ENDPOINT": "unix:///var/run/docker.sock", "CORE_VM_DOCKER_HOSTCONFIG_NETWORKMODE": project,
               "CORE_CHAINCODE_BUILDER": "hyperledger/fabric-ccenv:" + version,
               "CORE_CHAINCODE_GOLANG_RUNTIME": "hyperledger/fabric-baseos:" + version}
        services[name] = {"image": "hyperledger/fabric-peer:" + version, "environment": env,
                          "ports": [f"127.0.0.1:{port}:{port}"],
                          "volumes": [f"./crypto/peerOrganizations/{domain}/peers/{name}:/node:ro",
                                      volume + ":/var/hyperledger/production", "/var/run/docker.sock:/var/run/docker.sock"]}
    return {"services": services, "volumes": volumes, "networks": {"default": {"name": project}}}
