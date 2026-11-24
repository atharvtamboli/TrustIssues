#!/usr/bin/env python3
"""Build a self-contained portable release with checksums."""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--os', choices=['windows', 'linux', 'darwin'], required=True)
    parser.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    args = parser.parse_args()
    version = (ROOT / 'VERSION').read_text().strip()
    name = f'trustissues-{version}-{args.os}-{args.arch}'
    output = ROOT / 'dist'
    output.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='trustissues-build-') as temporary:
        stage = Path(temporary) / name
        stage.mkdir()
        exe = 'trustissues.exe' if args.os == 'windows' else 'trustissues'
        env = dict(os.environ, GOOS=args.os, GOARCH=args.arch, CGO_ENABLED='0')
        subprocess.run(['go', 'build', '-trimpath', '-ldflags=-s -w',
                        '-o', str(stage / exe), '.'], cwd=ROOT, env=env, check=True)
        for file in ['README.md', 'CHANGELOG.md', 'VERSION', 'CONTRIBUTING.md', 'SECURITY.md',
                     'docs/RELEASE_NOTES.md', 'docs/RELEASE_READINESS.md',
                     'network/CREDENTIALS.md', 'network/README.md']:
            source = ROOT / file
            if source.is_file():
                destination = stage / file
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)
        for source in (ROOT / 'docs' / 'screenshots').glob('*.webp'):
            destination = stage / source.relative_to(ROOT)
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, destination)
        shutil.copytree(ROOT / 'web', stage / 'web', ignore=shutil.ignore_patterns('*.html','*.js','*.css'))
        if args.os == 'windows':
            for file in ['Start-TrustIssues.ps1', 'Install-TrustIssues.ps1', 'Add-TrustIssuesUser.ps1']:
                shutil.copy2(ROOT / 'scripts' / file, stage / file)
        if args.os == 'windows':
            archive = output / (name + '.zip')
            with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
                for file in stage.rglob('*'):
                    if file.is_file():
                        bundle.write(file, file.relative_to(stage.parent))
        else:
            archive = output / (name + '.tar.gz')
            with tarfile.open(archive, 'w:gz') as bundle:
                bundle.add(stage, arcname=name)
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    archive.with_name(archive.name + '.sha256').write_text(digest + '  ' + archive.name + '\n')
    print(archive)

if __name__ == '__main__':
    main()

