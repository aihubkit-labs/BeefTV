#!/usr/bin/env python3
"""Verify draft uploads and reconcile GitHub's normalized download filenames."""
import argparse
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import subprocess


def reconcile(directory, assets, notes):
    local = sorted(p for p in directory.iterdir() if p.name != 'SHA256SUMS.txt')
    remote = [a for a in assets if a['name'] != 'SHA256SUMS.txt']
    if len(remote) != len(local):
        raise ValueError('Uploaded release asset count differs from the build inventory')
    by_content = defaultdict(list)
    for asset in remote:
        name = asset['name']
        if not name or any(c in name for c in '/\\\r\n') or name in ('.', '..') or asset['state'] != 'uploaded':
            raise ValueError('Invalid or incomplete release upload')
        by_content[(asset['digest'], asset['size'])].append(asset)
    mapping, checksums = {}, []
    for path in local:
        with path.open('rb') as stream:
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        candidates = by_content[('sha256:' + digest, path.stat().st_size)]
        matching = next((a for a in candidates if a['name'] == path.name), None)
        if matching is None:
            if len(candidates) != 1:
                raise ValueError('Cannot verify uploaded bytes for ' + path.name)
            matching = candidates[0]
        candidates.remove(matching)
        mapping[path.name] = matching['name']
        checksums.append(f"{digest}  {matching['name']}\n")
    if len(set(mapping.values())) != len(local):
        raise ValueError('GitHub normalized two asset names to the same filename')
    (directory / 'SHA256SUMS.txt').write_text(''.join(checksums), encoding='utf-8')
    # Replace only file references; preserve the platform's original display name.
    for original, uploaded in mapping.items():
        notes = notes.replace('`' + original + '`', '`' + uploaded + '`')
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
