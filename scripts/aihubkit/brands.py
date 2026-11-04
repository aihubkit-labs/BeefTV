#!/usr/bin/env python3
"""Resolve the operational registry once, then build each brand from that snapshot."""
import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import urllib.request
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[2]
REGISTRY = Path(__file__).with_name('brands.json')


def https(value, origin=False):
    if not isinstance(value, str) or any(c in value for c in '\r\n\\'):
        raise ValueError('Invalid HTTPS address')
    u = urlsplit(value.strip())
    if u.scheme != 'https' or not u.hostname or u.username or u.password:
        raise ValueError('Brand addresses require HTTPS without credentials')
    if origin and (u.path not in ('', '/') or u.query or u.fragment):
        raise ValueError('Brand origins must not contain paths, queries or fragments')
    return f'https://{u.netloc}' if origin else value.strip()


def registry(path=REGISTRY):
    brands = json.loads(path.read_text(encoding='utf-8'))['brands']
    if not brands:
        raise ValueError('Brand registry is empty')
    ids, origins = set(), set()
    for brand in brands:
        key = brand['id']
        if not re.fullmatch('[a-z][a-z0-9-]{0,31}', key) or key in ids:
            raise ValueError('Brand IDs must be unique stable lowercase identifiers')
        brand['origin'] = https(brand['origin'], True)
        if brand['origin'] in origins:
            raise ValueError('Each brand must have its own origin')
        ids.add(key)
        origins.add(brand['origin'])
    return brands


def metadata(brand, envelope):
    if envelope.get('success') is not True:
        raise ValueError(f"{brand['id']}: status did not succeed")
    data = envelope['data']
    name = data['system_name'].strip()
    if not name or len(name) > 100 or any(ord(c) < 32 for c in name):
        raise ValueError('Invalid platform name')
    app = name + 'TV'
    filename = re.sub(r'[<>:"/\\|?*\x00-\x1f]', '_', app).rstrip('. ')
    return dict(brand, platformName=name, appName=app, artifactName=filename,
                logoUrl=https(data['logo']), websiteUrl=https(data['server_address']),
                wechatUrl=https(data['wechat_qrcode']) if data.get('wechat_qrcode') else '',
                apiOrigin=https(data['api_server_address'], True), bundledLogo='/aihubkit-brand/logo.png')


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *_args, **_kwargs):
        raise ValueError('Brand metadata and logo must use their final HTTPS URLs')


def download(url, limit):
    req = urllib.request.Request(url, headers={'User-Agent': 'aihubkit-brand-builder', 'Accept': '*/*'})
    with urllib.request.build_opener(NoRedirect()).open(req, timeout=30) as response:
        if response.status != 200:
            raise ValueError('Brand resource download failed')
        body = response.read(limit + 1)
    if len(body) > limit:
        raise ValueError('Brand resource exceeds size limit')
    return body


def snapshot(output, selected=None):
    from PIL import Image, ImageOps
    brands = registry()
    if selected:
        brands = [x for x in brands if x['id'] == selected]
        if not brands:
            raise ValueError('Unknown brand ID')
    resolved, names = [], set()
    for entry in brands:
        brand = metadata(entry, json.loads(download(entry['origin'] + '/api/status', 1 << 20)))
        if brand['artifactName'].casefold() in names:
            raise ValueError('Brands returned duplicate application names; correct domain branding before release')
        names.add(brand['artifactName'].casefold())
        raw = download(brand['logoUrl'], 8 << 20)
        with Image.open(io.BytesIO(raw)) as source:
            if min(source.size) < 32 or max(source.size) > 8192:
                raise ValueError('Brand icon dimensions must be between 32 and 8192 pixels')
            icon = ImageOps.contain(source.convert('RGBA'), (1024, 1024))
        canvas = Image.new('RGBA', (1024, 1024))
        canvas.alpha_composite(icon, ((1024-icon.width)//2, (1024-icon.height)//2))
        directory = output / brand['id']
        directory.mkdir(parents=True, exist_ok=True)
        canvas.save(directory / 'logo.png')
        brand['logoSHA256'] = hashlib.sha256((directory / 'logo.png').read_bytes()).hexdigest()
        (directory / 'brand.json').write_text(json.dumps(brand, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
        resolved.append(brand)
    (output / 'brands.json').write_text(json.dumps({'brands': resolved}, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    return resolved


def load_snapshot(directory):
    brand = json.loads((directory / 'brand.json').read_text(encoding='utf-8'))
    entry = next((x for x in registry() if x['id'] == brand['id']), None)
    if not entry or entry['origin'] != brand['origin']:
        raise ValueError('Snapshot does not match the operational registry')
    validated = metadata(entry, {'success': True, 'data': dict(system_name=brand['platformName'], logo=brand['logoUrl'], server_address=brand['websiteUrl'], wechat_qrcode=brand['wechatUrl'], api_server_address=brand['apiOrigin'])})
    if any(brand[k] != v for k, v in validated.items()):
        raise ValueError('Invalid brand snapshot')
    if hashlib.sha256((directory / 'logo.png').read_bytes()).hexdigest() != brand['logoSHA256']:
        raise ValueError('Brand icon checksum mismatch')
    return brand


def environment(directory):
    brand = load_snapshot(directory)
    install = brand['id'] + 'TV'
    encode = lambda v: base64.b64encode(v.encode()).decode()
    snapshot_value = encode(json.dumps(brand, ensure_ascii=False, separators=(',', ':')))
    flags = {
        'infinite-canvas/backend/internal/integrations/aihubkit/branding.Snapshot': snapshot_value,
        'infinite-canvas/backend/internal/buildinfo.ProductNameBase64': encode(brand['appName']),
        'infinite-canvas/backend/internal/buildinfo.ProductScheme': brand['id'] + 'tv',
        'infinite-canvas/backend/internal/buildinfo.DataDirectory': install,
    }
    return {
        'BEEFTV_RELEASE_VERSION': (ROOT / 'AIHUBKIT_VERSION').read_text().strip(),
        'BEEFTV_EXTRA_LDFLAGS': ' '.join('-X '+k+'='+v for k, v in flags.items()),
        'BEEFTV_PRODUCT_NAME': brand['appName'], 'BEEFTV_ARTIFACT_NAME': brand['artifactName'],
        'BEEFTV_INSTALL_NAME': install, 'BEEFTV_PACKAGE_ID': brand['id']+'tv',
        'BEEFTV_APP_ICON': str((directory / 'logo.png').resolve()),
        'BEEFTV_PRODUCT_ID': 'com.aihubkit.'+brand['id'],
        'BEEFTV_UPDATER_PUBLIC_KEY': '',
    }


def build(directory, target):
    values = environment(directory)
    config = ROOT / 'backend/cmd/desktop/wails.json'
    original = config.read_bytes()
    public = ROOT / 'web/public/aihubkit-brand'
    if public.exists():
        raise ValueError('Generated brand assets already exist; use a clean build checkout')
    public.mkdir()
    try:
        shutil.copy2(directory / 'logo.png', public / 'logo.png')
        settings = json.loads(original)
        settings['info']['productName'] = values['BEEFTV_PRODUCT_NAME']
        settings['info']['protocols'] = [{'scheme': values['BEEFTV_PACKAGE_ID'], 'description': values['BEEFTV_PRODUCT_NAME'] + ' skill link', 'role': 'Viewer'}]
        if target == 'windows/amd64':
            settings['outputfilename'] = values['BEEFTV_INSTALL_NAME']
        config.write_text(json.dumps(settings, ensure_ascii=False, indent=2), encoding='utf-8')
        env = dict(os.environ, **values, BEEFTV_WAILS_PLATFORM=target)
        if target == 'windows/amd64':
            command = ['powershell', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', str(ROOT / 'scripts/build-beeftv-windows-release.ps1')]
        else:
            command = ['bash', str(ROOT / 'scripts/build-beeftv-release.sh')]
        subprocess.run(command, env=env, cwd=ROOT, check=True)
        out = ROOT / 'backend/cmd/desktop/build/bin'
        if target.startswith('darwin/'):
            (out / 'BeefTV.app').rename(out / (values['BEEFTV_INSTALL_NAME']+'.app'))
        elif target.startswith('linux/'):
            (out / 'BeefTV-linux').rename(out / (values['BEEFTV_INSTALL_NAME']+'-linux'))
    finally:
        config.write_bytes(original)
        shutil.rmtree(public)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('snapshot')
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--brand')
    p = sub.add_parser('env')
    p.add_argument('--snapshot', type=Path, required=True)
    p.add_argument('--github-env', type=Path, required=True)
    p = sub.add_parser('build')
    p.add_argument('--snapshot', type=Path, required=True)
    p.add_argument('--target', required=True, choices=['darwin/arm64', 'darwin/amd64', 'windows/amd64', 'linux/amd64'])
    args = parser.parse_args()
    if args.command == 'snapshot':
        result = snapshot(args.output, args.brand)
        print(json.dumps([x['id'] for x in result]))
    elif args.command == 'env':
        with args.github_env.open('a', encoding='utf-8') as stream:
            for key, value in environment(args.snapshot).items():
                if '\n' in value or '\r' in value:
                    raise ValueError('Multiline build environment value')
                stream.write(key+'='+value+'\n')
    else:
        build(args.snapshot, args.target)


if __name__ == '__main__':
    main()
