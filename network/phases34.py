"""Deploy the credential contract and provision signing profiles for the local Fabric pilot."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

import phase2 as net

PROJECT = net.HERE.parent
NAMES = {1: "Demo University", 2: "Partner College"}


def signer(org, directory):
    msp, endpoint, tls, admin = net.identities(org)
    group = admin.parents[1]
    leaf = directory / ("issuer-" + str(org))
    if not leaf.exists():
        with tempfile.TemporaryDirectory(dir=directory) as tmp:
            src = Path(tmp)
            ca_cert, = (group / "ca").glob("*-cert.pem")
            ca_key, = (group / "ca").glob("*_sk")
            net.run("go", "run", "./network/enroll", ca_cert, ca_key, src,
                    "fatbot-issuer-" + str(org), cwd=PROJECT)
            src.rename(leaf)
    return dict(MSP=msp, Endpoint=endpoint, ServerName=net.peer_name(org), CA=str(tls),
                Cert=str(leaf / "signer.pem"), Key=str(leaf / "signer.key"))


def install():
    directory = net.ROOT / "credential-service"
    directory.mkdir(exist_ok=True, mode=0o700)
    if not (net.ROOT / "credentials.block").exists():
        raise RuntimeError("First run network/phase2.py up.")
    source = PROJECT / "chaincode/credential"
    fingerprint = hashlib.sha256()
    for p in sorted([*source.rglob("*.go"), source / "go.mod"]):
        fingerprint.update(p.relative_to(source).as_posix().encode() + b"\0" + p.read_bytes())
    label = directory / "sequence-1.sha256"
    if label.exists() and label.read_text() != fingerprint.hexdigest():
        raise RuntimeError("Contract source changed since packaging sequence 1. Use a new lifecycle sequence before redeploying.")
    with tempfile.TemporaryDirectory(dir=directory) as tmp:
        stage = Path(tmp) / "chaincode"
        shutil.copytree(source, stage, ignore=shutil.ignore_patterns("vendor"))
        net.run("go", "mod", "tidy", cwd=stage)
        net.run("go", "test", "./...", cwd=stage)
        net.run("go", "mod", "vendor", cwd=stage, timeout=600)
        label.write_text(fingerprint.hexdigest())
        net.deploy("credential", stage, "AND('Org1MSP.peer','Org2MSP.peer')")
        gw_src = Path(tmp) / "gateway"
        shutil.copytree(net.HERE / "gateway", gw_src)
        net.run("go", "mod", "tidy", cwd=gw_src)
        net.run("go", "build", "-o", directory / "gateway.next", ".", cwd=gw_src, timeout=600)
        (directory / "gateway.next").replace(directory / "gateway")
    issuers = {}
    for i, name in NAMES.items():
        profile = directory / f"org{i}.json"
        data = signer(i, directory)
        net.save(profile, data)
        issuers[name] = dict(Profile=str(profile), MSP=data["MSP"])
    msp, endpoint, ca, admin = net.identities(2)
    user = admin.parent / ("User1@" + net.ORGS[2][1]) / "msp"
    cert, = (user / "signcerts").glob("*")
    key, = (user / "keystore").glob("*")
    reader = directory / "partner-reader.json"
    net.save(reader, dict(MSP=msp, Endpoint=endpoint, ServerName=net.peer_name(2), CA=str(ca), Cert=str(cert), Key=str(key)))
    config = directory / "config.json"
    net.save(config, dict(Executable=str(directory / "gateway"), Reader=str(reader), Issuers=issuers))
    print(f"Fabric application configuration: {config}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("deploy", "prove"))
    action = parser.parse_args().action
    net.doctor()
    import fcntl
    os.umask(0o077)
    if not net.ROOT.is_dir():
        raise RuntimeError("First run network/phase2.py up.")
    with (net.ROOT.parent / ("." + net.ROOT.name + ".lock")).open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if action == "deploy":
            install()
        else:
            config = net.ROOT / "credential-service/config.json"
            if not config.exists():
                raise RuntimeError("Deploy the credential service first.")
            env = dict(os.environ, FATBOT_LIVE_TEST_CONFIG=str(config))
            net.run("go", "test", "-count=1", "-v", "-run", "^TestLiveFabricPortals$", ".", cwd=PROJECT, env=env, timeout=360)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, subprocess.TimeoutExpired) as exc:
        sys.exit(str(exc))
