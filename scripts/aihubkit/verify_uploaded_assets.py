#!/usr/bin/env python3
"""Verify draft upload names and bytes before making a release public."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def reconcile(directory, assets, notes):
    local = sorted(p for p in directory.iterdir() if p.name != 'SHA256SUMS.txt')
    remote = [a for a in assets if a['name'] != 'SHA256SUMS.txt']
    if len(remote) != len(local):
        raise ValueError('Uploaded release asset count differs from the build inventory')
    by_name = {}
    for asset in remote:
        name = asset['name']
        if not name or any(c in name for c in '/\\\r\n') or name in ('.', '..') or asset['state'] != 'uploaded':
            raise ValueError('Invalid or incomplete release upload')
        if name in by_name:
            raise ValueError('Duplicate release filename: ' + name)
        by_name[name] = asset
    if set(by_name) != {path.name for path in local}:
        raise ValueError('Uploaded release filenames differ from the build inventory; publication is blocked')
    checksums = []
    for path in local:
        with path.open('rb') as stream:
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        matching = by_name[path.name]
        if matching['digest'] != 'sha256:' + digest or matching['size'] != path.stat().st_size:
            raise ValueError('Cannot verify uploaded bytes for ' + path.name)
        checksums.append(f"{digest}  {path.name}\n")
    (directory / 'SHA256SUMS.txt').write_text(''.join(checksums), encoding='utf-8')
    return notes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--assets-dir', type=Path, required=True)
    parser.add_argument('--notes', type=Path, required=True)
    args = parser.parse_args()
    release = json.loads(subprocess.check_output(['gh', 'release', 'view', args.tag, '--json', 'isDraft,assets'], encoding='utf-8'))
    if not release['isDraft']:
        raise ValueError('Only draft release uploads may be reconciled')
    notes = reconcile(args.assets_dir, release['assets'], args.notes.read_text(encoding='utf-8'))
    args.notes.write_text(notes, encoding='utf-8')


if __name__ == '__main__':
    main()
