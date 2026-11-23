"""Local Fabric infrastructure for FatBot. Run with Python 3.9+ in WSL2/Linux."""
import argparse
import base64
import hashlib
import json
import os
import platform
import re
import secrets
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.request
from pathlib import Path

from topology import ADMIN_PORT, CHANNEL, ORDERER, ORDER_PORT, ORGS, PROJECT, channel_config, compose_config, crypto_config, peer_name

HERE = Path(__file__).resolve().parent
VERSIONS = json.loads((HERE / "versions.json").read_text())
ROOT = Path(os.environ.get("FATBOT_FABRIC_HOME", Path.home() / ".local/share/fatbot-fabric-phase2")).resolve()
COMPOSE_PROJECT = PROJECT + "_" + hashlib.sha256(str(ROOT).encode()).hexdigest()[:12]
CRYPTO = ROOT / "crypto"
SAMPLES = ROOT / "samples"
CC = "basic"
TOOLS = ("peer", "osnadmin", "cryptogen", "configtxgen", "discover")


def run(*args, env=None, cwd=None, capture=False, timeout=180):
    result = subprocess.run([str(s) for s in args], cwd=cwd, env=env, text=True,
                            capture_output=capture, timeout=timeout)
    if result.returncode:
        detail = (result.stderr or result.stdout or "").strip()
        raise RuntimeError(f"{args[0]} {args[1]} failed ({result.returncode}): {detail}")
    return result.stdout.strip() if capture else None


def save(path, obj):
    with tempfile.NamedTemporaryFile("w", dir=path.parent, delete=False) as f:
        json.dump(obj, f, indent=2)
        f.write("\n")
    os.replace(f.name, path)


def doctor():
    issues = []
    if platform.system() != "Linux":
        issues.append("Run inside WSL2/Linux to use the Linux peer binaries.")
    for command in ("docker", "go", "git"):
        if not shutil.which(command):
            issues.append(f"Missing executable: {command}")
    if shutil.which("docker"):
        for args in (("docker", "compose", "version"), ("docker", "info", "--format", "{{.OSType}}")):
            try:
                value = run(*args, capture=True, timeout=15)
                print(value)
                if args[1] == "info" and value != "linux":
                    issues.append("Select a Linux Docker engine.")
            except (RuntimeError, subprocess.TimeoutExpired) as exc:
                issues.append(str(exc))
    if issues:
        raise RuntimeError("\n".join(issues))


def tools_install():
    version = VERSIONS["fabric"]
    arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine())
    if not arch:
        raise RuntimeError("Unsupported Fabric binary architecture")
    name = f"hyperledger-fabric-linux-{arch}-{version}.tar.gz"
    api = f"https://api.github.com/repos/hyperledger/fabric/releases/tags/v{version}"
    with urllib.request.urlopen(api, timeout=45) as response:
        assets = json.load(response)["assets"]
    asset = next((a for a in assets if a["name"] == name), None)
    if not asset or not re.fullmatch(r"sha256:[a-f0-9]{64}", asset.get("digest") or ""):
        raise RuntimeError(f"No release SHA-256 digest available for {name}")
    with tempfile.TemporaryFile() as archive:
        with urllib.request.urlopen(asset["browser_download_url"], timeout=120) as response:
            shutil.copyfileobj(response, archive)
        archive.seek(0)
        checksum = hashlib.sha256()
        for chunk in iter(lambda: archive.read(1024 * 1024), b""):
            checksum.update(chunk)
        if "sha256:" + checksum.hexdigest() != asset["digest"]:
            raise RuntimeError("Fabric binary archive checksum mismatch")
        archive.seek(0)
        required = {"bin/" + tool for tool in TOOLS} | {"config/core.yaml"}
        with tarfile.open(fileobj=archive, mode="r:gz") as bundle:
            for member in bundle:
                name = member.name.removeprefix("./")
                if not member.isfile() or name not in required:
                    continue
                path = ROOT / name
                path.parent.mkdir(exist_ok=True)
                with bundle.extractfile(member) as src, path.open("wb") as dst:
                    shutil.copyfileobj(src, dst)
                path.chmod(0o700 if name.startswith("bin/") else 0o600)
                required.remove(name)
        if required:
            raise RuntimeError(f"Missing binary archive entries: {sorted(required)}")


def prepare():
    ROOT.mkdir(parents=True, exist_ok=True)
    if not (ROOT / "versions.json").exists():
        if any(ROOT.iterdir()) and not (ROOT / ".preparing").exists():
            raise RuntimeError("Select an empty FATBOT_FABRIC_HOME for first setup.")
        (ROOT / ".preparing").touch()
        tools_install()
        if not SAMPLES.exists():
            run("git", "clone", "https://github.com/hyperledger/fabric-samples.git", SAMPLES, timeout=600)
        run("git", "-C", SAMPLES, "checkout", "--detach", VERSIONS["samples_commit"])
        run("go", "mod", "vendor", cwd=SAMPLES / "asset-transfer-basic/chaincode-go", timeout=600)
        run("go", "mod", "download", "github.com/hyperledger/fabric-gateway@" + VERSIONS["gateway_go"],
            cwd=SAMPLES / "asset-transfer-basic/application-gateway-go", timeout=300)
        for image in ("peer", "orderer", "ccenv", "baseos"):
            run("docker", "pull", f"hyperledger/fabric-{image}:{VERSIONS['fabric']}", timeout=600)
        save(ROOT / "versions.json", VERSIONS)
        (ROOT / ".preparing").unlink()
    if json.loads((ROOT / "versions.json").read_text()) != VERSIONS:
        raise RuntimeError("Existing network uses different versions; select its original configuration.")
    if not run("git", "-C", SAMPLES, "rev-parse", "HEAD", capture=True).startswith(VERSIONS["samples_commit"]):
        raise RuntimeError("Sample contract checkout differs from the pinned revision")
    if not CRYPTO.exists():
        save(ROOT / "crypto.yaml", crypto_config())
        with tempfile.TemporaryDirectory(dir=ROOT) as tmp:
            run(ROOT / "bin/cryptogen", "generate", "--config", ROOT / "crypto.yaml", "--output", Path(tmp) / "crypto")
            (Path(tmp) / "crypto").rename(CRYPTO)
    save(ROOT / "compose.yaml", compose_config(VERSIONS["fabric"], COMPOSE_PROJECT))
    save(ROOT / "configtx.yaml", channel_config(CRYPTO))
    if not (ROOT / "credentials.block").exists():
        run(ROOT / "bin/configtxgen", "-configPath", ROOT, "-profile", "FatBot",
            "-channelID", CHANNEL, "-outputBlock", ROOT / "creating.block")
        os.replace(ROOT / "creating.block", ROOT / "credentials.block")


def compose(*args, **kwargs):
    return run("docker", "compose", "-p", COMPOSE_PROJECT, "-f", ROOT / "compose.yaml", *args, **kwargs)


def identities(i):
    org, domain, port = ORGS[i]
    group = CRYPTO / "peerOrganizations" / domain
    return (org + "MSP", f"localhost:{port}", group / "peers" / peer_name(i) / "tls/ca.crt",
            group / "users" / ("Admin@" + domain))


def peer_env(i):
    msp, address, ca, user = identities(i)
    env = dict(os.environ, FABRIC_CFG_PATH=str(ROOT / "config"), CORE_PEER_LOCALMSPID=msp,
               CORE_PEER_ADDRESS=address, CORE_PEER_TLS_ENABLED="true", CORE_PEER_TLS_ROOTCERT_FILE=str(ca),
               CORE_PEER_MSPCONFIGPATH=str(user / "msp"))
    return env


def peer(i, *args, timeout=180):
    return run(ROOT / "bin/peer", *args, env=peer_env(i), capture=True, timeout=timeout)


def order_tls():
    return CRYPTO / "ordererOrganizations/fatbot.test/orderers" / ORDERER / "tls"


def order_flags():
    return ["-o", f"localhost:{ORDER_PORT}", "--tls", "--cafile", order_tls() / "ca.crt"]


def retry(fn, seconds=90):
    end = time.monotonic() + seconds
    while True:
        try:
            return fn()
        except (RuntimeError, ValueError, subprocess.TimeoutExpired) as exc:
            if time.monotonic() >= end:
                raise RuntimeError(f"Timed out: {exc}") from exc
            time.sleep(2)


def setup_channel():
    tls = order_tls()
    def admin(*args):
        result = run(ROOT / "bin/osnadmin", "channel", *args, "-o", f"localhost:{ADMIN_PORT}",
                     "--ca-file", tls / "ca.crt", "--client-cert", tls / "server.crt",
                     "--client-key", tls / "server.key", "--no-status", capture=True)
        return json.loads(result)
    channels = retry(lambda: admin("list"))
    if CHANNEL not in [c["name"] for c in channels.get("channels") or []]:
        admin("join", "--channelID", CHANNEL, "--config-block", ROOT / "credentials.block")
    for i in ORGS:
        listed = retry(lambda: peer(i, "channel", "list"))
        if CHANNEL not in listed.splitlines():
            peer(i, "channel", "join", "-b", ROOT / "credentials.block")
        retry(lambda: info(i))


def deploy(name=CC, source=None, policy="OR('Org1MSP.peer','Org2MSP.peer')"):
    source = source or (SAMPLES / "asset-transfer-basic/chaincode-go")
    flags = ["--channelID", CHANNEL, "--name", name, "--version", "1.0", "--sequence", "1",
             "--signature-policy", policy]
    package = ROOT / (name + ".tar.gz")
    if not package.exists():
        peer(1, "lifecycle", "chaincode", "package", ROOT / "packaging.tar.gz", "--path",
             source, "--lang", "golang", "--label", f"fatbot-{name}_1",
             timeout=600)
        os.replace(ROOT / "packaging.tar.gz", package)
    pkgid = peer(1, "lifecycle", "chaincode", "calculatepackageid", package)
    for i in ORGS:
        installed = json.loads(peer(i, "lifecycle", "chaincode", "queryinstalled", "--output", "json"))
        if pkgid not in [p["package_id"] for p in installed.get("installed_chaincodes", [])]:
            peer(i, "lifecycle", "chaincode", "install", package, timeout=600)
    committed = json.loads(peer(1, "lifecycle", "chaincode", "querycommitted", "-C", CHANNEL, "--output", "json"))
    if any(d["name"] == name and str(d["sequence"]) == "1" for d in committed.get("chaincode_definitions", [])):
        return
    for i in ORGS:
        ready = json.loads(peer(i, "lifecycle", "chaincode", "checkcommitreadiness", *flags, "--output", "json"))
        if not ready.get("approvals", {}).get(identities(i)[0], False):
            peer(i, "lifecycle", "chaincode", "approveformyorg", *order_flags(), *flags, "--package-id", pkgid)
    targets = []
    for i in ORGS:
        _, address, ca, _ = identities(i)
        targets.extend(["--peerAddresses", address, "--tlsRootCertFiles", ca])
    peer(1, "lifecycle", "chaincode", "commit", *order_flags(), *flags, *targets)


def info(i):
    output = peer(i, "channel", "getinfo", "-c", CHANNEL)
    result = json.loads(output[output.index("{"):])
    digest = result.get("currentBlockHash") or result.get("current_block_hash") or ""
    if int(result["height"]) < 1 or len(base64.b64decode(digest, validate=True)) != 32:
        raise ValueError("Invalid peer chain information")
    return {"height": int(result["height"]), "hash_base64": digest}


def discovery(i):
    msp, addr, ca, user = identities(i)
    cert, = (user / "msp/signcerts").glob("*")
    key, = (user / "msp/keystore").glob("*")
    data = json.loads(run(ROOT / "bin/discover", "--peerTLSCA", ca, "--userKey", key,
                          "--userCert", cert, "--MSP", msp, "--tlsCert", user / "tls/client.crt",
                          "--tlsKey", user / "tls/client.key", "peers", "--channel", CHANNEL,
                          "--server", addr, capture=True))
    members = sorted((p["MSPID"], p["Endpoint"]) for p in data)
    for n, (org, _, port) in ORGS.items():
        if (org + "MSP", f"{peer_name(n)}:{port}") not in members:
            raise RuntimeError(f"Discovery from {addr} missing {org}")
    return members


def write(asset):
    _, addr, ca, _ = identities(1)
    args = json.dumps({"Args": ["CreateAsset", asset, "blue", "3", "FatBot", "100"]})
    # Fabric's CLI writes the VALID event and transaction ID on stderr.
    proc = subprocess.run([str(ROOT / "bin/peer"), "chaincode", "invoke", *order_flags(),
                           "-C", CHANNEL, "-n", CC, "-c", args, "--peerAddresses", addr,
                           "--tlsRootCertFiles", ca, "--waitForEvent", "--waitForEventTimeout", "60s"],
                          env=peer_env(1), stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=90)
    match = re.search(r"txid\s*\[([a-fA-F0-9]{64})\]\s+committed with status\s*\(VALID\)", proc.stdout)
    if proc.returncode or not match:
        raise RuntimeError(f"Could not confirm valid commit: {proc.stdout}")
    return match[1]


def same_state(asset):
    states = []
    for i in ORGS:
        raw = peer(i, "chaincode", "query", "-C", CHANNEL, "-n", CC,
                   "-c", json.dumps({"Args": ["ReadAsset", asset]}))
        states.append(json.loads(raw))
    values = {k.lower(): v for k, v in states[0].items()}
    expected = {"id": asset, "owner": "FatBot", "color": "blue", "size": 3, "appraisedvalue": 100}
    if states[0] != states[1] or any(values.get(k) != v for k, v in expected.items()):
        raise RuntimeError("Peer values differ from the submitted asset")
    heads = [info(i) for i in ORGS]
    if heads[0] != heads[1]:
        raise RuntimeError("Peer heights or current block hashes differ")
    return {"value": states[0], "peer_a": heads[0], "peer_b": heads[1]}


def volume_b():
    cid = compose("ps", "-q", peer_name(2), capture=True)
    mounts = json.loads(run("docker", "inspect", "-f", "{{json .Mounts}}", cid, capture=True))
    volume, = [m["Name"] for m in mounts if m["Type"] == "volume" and m["Destination"] == "/var/hyperledger/production"]
    return volume


def prove():
    discovery_results = {identities(i)[1]: retry(lambda: discovery(i)) for i in ORGS}
    first = "phase2-" + secrets.token_hex(12)
    first_tx = write(first)
    before = retry(lambda: same_state(first))
    volume = volume_b()
    second = "phase2-" + secrets.token_hex(12)
    try:
        compose("stop", peer_name(2))
        if compose("ps", "--status", "running", "-q", peer_name(2), capture=True):
            raise RuntimeError("Partner peer still running")
        catchup_tx = write(second)
        ahead = info(1)
        if ahead["height"] <= before["peer_b"]["height"]:
            raise RuntimeError("University ledger did not advance during the outage")
    finally:
        compose("start", peer_name(2))
    after = retry(lambda: same_state(second))
    if volume_b() != volume or after["peer_b"]["height"] < ahead["height"]:
        raise RuntimeError("Partner catch-up or persistent ledger volume check failed")
    report = {"channel": CHANNEL, "chaincode": CC, "versions": VERSIONS,
              "discovery": discovery_results, "initial_tx": first_tx, "initial_commit": before,
              "offline_tx": catchup_tx, "university_while_partner_stopped": ahead, "after_restart": after,
              "partner_volume": volume, "utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}
    path = ROOT / ("proof-" + first + ".json")
    save(path, report)
    print(f"Acceptance passed: {path}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["doctor", "up", "prove"])
    action = parser.parse_args().action
    doctor()
    if action == "doctor":
        return
    os.umask(0o077)
    ROOT.mkdir(parents=True, exist_ok=True)
    import fcntl
    with (ROOT.parent / ("." + ROOT.name + ".lock")).open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if action == "up":
            prepare()
            print(f"Compose project: {COMPOSE_PROJECT} | runtime: {ROOT}", flush=True)
            compose("up", "-d")
            setup_channel()
            deploy()
        else:
            if not (ROOT / "compose.yaml").is_file():
                raise RuntimeError("Run up before prove.")
            prove()


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as exc:
        sys.exit(str(exc))
