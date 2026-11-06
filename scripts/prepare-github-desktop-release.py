#!/usr/bin/env python3
"""Validate a tag release and its complete desktop download inventory."""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
VERSION_FILE = ROOT / 'AIHUBKIT_VERSION'
CHANGELOG_FILE = ROOT / 'AIHUBKIT_CHANGELOG.md'
TARGETS = {
    'windows-amd64': '-setup.exe',
    'linux-amd64': '.deb',
    'darwin-arm64': '.dmg',
    'darwin-amd64': '.dmg',
}


def release_notes(tag, version, changelog, brands=None):
    if not re.fullmatch(r'aihubkit-v[0-9]+\.[0-9]+\.[0-9]+', tag):
        raise ValueError('Desktop installers require an independent tag: aihubkit-vMAJOR.MINOR.PATCH')
    app_version = tag.removeprefix('aihubkit-')
    if app_version != version.strip():
        raise ValueError(f'Tag {tag} does not match AIHUBKIT_VERSION={version.strip()}')
    lines = changelog.splitlines()
    heading = f'## {app_version}'
    if heading not in lines:
        raise ValueError(f'AIHUBKIT_CHANGELOG.md is missing {heading}')
    body = []
    for line in lines[lines.index(heading) + 1:]:
        if line.startswith('## '):
            break
        body.append(line)
    changes = '\n'.join(body).strip()
    if not changes:
        raise ValueError(f'AIHUBKIT_CHANGELOG.md has no release notes for {tag}')
    notes = f'''{changes}

## 下载与安装

| 系统 | 安装包 |
| --- | --- |
| Windows x64 | `BeefTV-{app_version}-windows-amd64-setup.exe` |
| Ubuntu 24.04 x64 | `BeefTV-{app_version}-linux-amd64.deb` |
| macOS Apple Silicon | `BeefTV-{app_version}-darwin-arm64.dmg` |
| macOS Intel | `BeefTV-{app_version}-darwin-amd64.dmg` |

Windows 运行 Setup；macOS 打开 DMG 并拖入 Applications；Ubuntu 运行
`sudo apt install ./BeefTV-{app_version}-linux-amd64.deb`。
ZIP 用于便携运行，解压时保留全部文件。安装包包含前端、后端、CLI、助手运行时和官方插件，用户无需安装 Go、Bun 或 Node.js。

Windows 缺少 WebView2 时需按提示安装。Linux DEB 会通过 apt 安装系统依赖。
Windows 未做 Authenticode 签名；macOS 使用 ad hoc 签名，未做 Apple 公证，首次运行可能需要系统放行。
本次 tag 构建不启用官方应用内自动更新，升级时请下载并安装新版。

`SHA256SUMS.txt` 包含所有下载文件的 SHA-256。媒体组件的对应源码包一并提供。
'''
    if brands:
        start, end = notes.index('| 系统 |'), notes.index('ZIP 用于')
        rows = ['| 品牌 | 绑定域名 | 平台 | 安装包 |', '| --- | --- | --- | --- |']
        for brand in brands:
            for target, suffix in TARGETS.items():
                rows.append(f"| {brand['appName']} | {brand['origin']} | {target} | `{brand['artifactName']}-{app_version}-{target}{suffix}` |")
        notes = notes[:start] + '\n'.join(rows) + '\n\nWindows 运行 Setup；macOS 将应用拖入 Applications；Ubuntu 使用 `sudo apt install ./下载的文件.deb`。\n' + notes[end:]
    return notes


def prepare_assets(directory, tag, brands=None):
    app_version = tag.removeprefix('aihubkit-')
    names = [brand['artifactName'] for brand in brands] if brands else ['BeefTV']
    expected = {f'{name}-{app_version}-{target}{suffix}' for name in names for target, suffix in TARGETS.items()}
    expected |= {f'{name}-{app_version}-{target}.zip' for name in names for target in TARGETS}
    expected |= {f'BeefTV-{app_version}-ffprobe-{target}-sources.tar.gz'
                 for target in TARGETS if target.startswith('darwin-')}
    expected.add(f'BeefTV-{app_version}-ffmpeg-wasm-sources.tar.gz')
    for name in sorted(expected):
        path = directory / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError(f'Missing or empty release asset: {name}')
    unexpected = {path.name for path in directory.iterdir()} - expected - {'SHA256SUMS.txt'}
    if unexpected:
        raise ValueError(f'Unexpected release assets: {", ".join(sorted(unexpected))}')
    checksums = []
    for name in sorted(expected):
        with (directory / name).open('rb') as stream:
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        checksums.append(f'{digest}  {name}\n')
    (directory / 'SHA256SUMS.txt').write_text(''.join(checksums), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--notes', required=True, type=Path)
    parser.add_argument('--assets-dir', type=Path)
    parser.add_argument('--brands', type=Path)
    args = parser.parse_args()
    brands = None
    if args.brands:
        from aihubkit.brands import registry
        brands = json.loads(args.brands.read_text(encoding='utf-8'))['brands']
        if {(b['id'], b['origin']) for b in brands} != {(b['id'], b['origin']) for b in registry()} or len(brands) != len(registry()):
            raise ValueError('Release snapshot must contain every operational brand exactly once')
        if len({b['artifactName'].casefold() for b in brands}) != len(brands):
            raise ValueError('Duplicate brand artifact names')
    notes = release_notes(args.tag, VERSION_FILE.read_text(),
                          CHANGELOG_FILE.read_text(encoding='utf-8'), brands)
    if args.assets_dir:
        if not brands:
            raise ValueError('A formal release requires the frozen operational brand inventory')
        prepare_assets(args.assets_dir, args.tag, brands)
    args.notes.write_text(notes, encoding='utf-8')


if __name__ == '__main__':
    try:
        main()
    except ValueError as error:
        raise SystemExit(str(error)) from error
