#!/usr/bin/env python3
"""按运营清单，一键构建当前系统的全部品牌安装包。"""
import argparse
from contextlib import contextmanager
from datetime import datetime
import hashlib
from http.client import IncompleteRead
import json
import os
from pathlib import Path
import platform
import re
import runpy
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError, URLError
import zipfile

import brands

ROOT = Path(__file__).resolve().parents[2]


def run(args, env, cwd=None, capture=False):
    args = [arg.as_posix() if isinstance(arg, Path) else str(arg) for arg in args]
    # Windows resolves the initial executable before applying the child's PATH.
    args[0] = shutil.which(args[0], path=env.get('PATH')) or args[0]
    child_env = env.copy()
    if Path(args[0]).stem.lower() == 'powershell':
        child_env.pop('PSModulePath', None)
    return subprocess.run(args, cwd=cwd or ROOT, env=child_env, check=True,
                          capture_output=capture, text=True, encoding='utf-8')


def native_target():
    system, machine = platform.system(), platform.machine().lower()
    if system == 'Darwin' and machine == 'x86_64':
        hardware = subprocess.run(['/usr/sbin/sysctl', '-n', 'hw.optional.arm64'],
                                  capture_output=True, text=True, check=False)
        if hardware.returncode == 0 and hardware.stdout.strip() == '1':
            # Intel Anaconda under Rosetta reports x86_64 on an Apple Silicon host.
            print('检测到 Rosetta Python，切换到 macOS 原生 Python……', flush=True)
            os.execv('/usr/bin/arch', ['arch', '-arm64', '/usr/bin/python3', str(Path(__file__).resolve()), *sys.argv[1:]])
    if system == 'Windows':
        machine = os.environ.get('PROCESSOR_ARCHITEW6432', os.environ.get('PROCESSOR_ARCHITECTURE', machine)).lower()
    arch = {'arm64': 'arm64', 'aarch64': 'arm64', 'x86_64': 'amd64', 'amd64': 'amd64'}.get(machine)
    if system == 'Darwin' and arch:
        return 'darwin/' + arch
    if system in ('Windows', 'Linux') and arch == 'amd64':
        return system.lower() + '/amd64'
    raise ValueError(f'暂不支持 {system}/{machine}；支持 macOS arm64/amd64、Windows x64、Ubuntu 24.04 x64。')


def powershell(env):
    shell = shutil.which('pwsh', path=env.get('PATH')) or shutil.which('powershell', path=env.get('PATH'))
    if not shell:
        raise ValueError('缺少 PowerShell，请安装 PowerShell 或将 powershell.exe 加入 PATH。')
    return [shell, '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File']


def toolchain(target):
    env = os.environ.copy()
    windows = target.startswith('windows/')
    go_version = re.search(r'^go (\S+)$', (ROOT / 'backend/go.mod').read_text(), re.M).group(1)
    go_dir = env.get('BEEFTV_GO_DIR')
    if not go_dir and not windows:
        candidate = Path(env.get('GVM_ROOT', Path.home() / '.gvm')) / 'gos' / ('go' + go_version)
        if (candidate / 'bin/go').is_file():
            go_dir = str(candidate)
    if go_dir:
        env.update(BEEFTV_GO_DIR=go_dir, GOROOT=go_dir)
        env['PATH'] = str(Path(go_dir) / 'bin') + os.pathsep + env.get('PATH', '')

    if not shutil.which('bun', path=env.get('PATH')):
        raise ValueError('未找到 Bun，请先安装 Bun 并加入 PATH。')
    node_version = run([
        'bun', '-e', 'import { NODE_VERSION } from "./scripts/package-agent-host.mjs"; console.log(NODE_VERSION);',
    ], env, capture=True).stdout.strip()
    runtime = env.get('BEEFTV_NODE_RUNTIME')
    if not runtime:
        candidate = Path(env.get('NVM_DIR', Path.home() / '.nvm')) / 'versions/node' / ('v' + node_version)
        if not windows and (candidate / 'bin/node').is_file():
            runtime = str(candidate)
        else:
            node = shutil.which('node', path=env.get('PATH'))
            if not node:
                raise ValueError(f'请先运行 nvm install {node_version}，或设置 BEEFTV_NODE_RUNTIME。')
            node_directory = Path(node).resolve().parent
            runtime = str(node_directory if windows else node_directory.parent)
    env['BEEFTV_NODE_RUNTIME'] = runtime
    if windows:
        # Share the official compiler selection; MSYS2 also supplies the official plugin packager's tools.
        prefix = Path(env.get('MSYSTEM_PREFIX') or 'C:/msys64/ucrt64')
        env['MSYSTEM_PREFIX'] = str(prefix)
        env['CC'] = str(prefix / 'bin/gcc.exe')
        nsis = Path(env.get('ProgramFiles(x86)', 'C:/Program Files (x86)')) / 'NSIS'
        env['PATH'] = os.pathsep.join([str(prefix / 'bin'), str(prefix.parent / 'usr/bin'), str(nsis), env.get('PATH', '')])
    env['PATH'] = str(Path(runtime) if windows else Path(runtime) / 'bin') + os.pathsep + env.get('PATH', '')
    required = ['go', 'node', 'git', 'bash', 'zip']
    if windows:
        required += ['makensis', 'gcc']
    elif target.startswith('darwin/'):
        required += ['xcrun', 'codesign', 'hdiutil']
    else:
        required += ['gcc', 'pkg-config', 'dpkg-deb']
        release = dict(line.split('=', 1) for line in Path('/etc/os-release').read_text().splitlines() if '=' in line)
        if (release.get('ID', '').strip('"'), release.get('VERSION_ID', '').strip('"')) != ('ubuntu', '24.04'):
            raise ValueError('当前 DEB 构建基于 Ubuntu 24.04，请在该系统的 x64 主机上运行。')
    for tool in required:
        if not shutil.which(tool, path=env['PATH']):
            raise ValueError(f'缺少构建工具：{tool}。请按 aihubkit 平台适配文档安装本机工具链。')
    if env.get('BEEFTV_WAILS_PLATFORM', target) != target:
        raise ValueError('此入口仅打包本机架构，请取消 BEEFTV_WAILS_PLATFORM 后重试。')
    env.update(CGO_ENABLED='1', BEEFTV_SKIP_LOCAL_VERIFY='0', BEEFTV_WAILS_PLATFORM=target)
    env.setdefault('GOTOOLCHAIN', 'auto')
    # Binding generation must never open the user's installed workspace.
    env['CANVAS_DESKTOP_DATA_DIR'] = str(ROOT / '.local/project-workbench-debug')
    if windows:
        run(powershell(env) + [ROOT / 'scripts/assert-windows-dwarf5-cgo.ps1'], env)
    elif target.startswith('darwin/'):
        run(['xcrun', '--find', 'clang'], env)
    else:
        run(['pkg-config', '--exists', 'gtk+-3.0', 'webkit2gtk-4.1'], env)
    run(['bun', 'scripts/package-agent-host.mjs', target, '--verify-runtime'], env)
    run(['go', 'version'], env, ROOT / 'backend')
    return env


def python_environment(env, target):
    venv = ROOT / '.local/brand-build-venv'
    python = venv / ('Scripts/python.exe' if target.startswith('windows/') else 'bin/python3')
    if not python.is_file():
        run([sys.executable, '-m', 'venv', venv], env)
    run([python, '-m', 'pip', 'install', '--disable-pip-version-check', 'Pillow==11.3.0'], env)
    return python


def prepare_dependencies(env):
    run(['go', 'mod', 'download'], env, ROOT / 'backend')
    run(['bun', 'install', '--frozen-lockfile'], env, ROOT / 'web')
    cache = ROOT / '.local/cache'
    cache.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='brand-plugins-', dir=cache) as directory:
        source = ROOT / 'plugin-packages'
        stage = Path(directory) / 'plugin-packages'
        shutil.copytree(source, stage, ignore=shutil.ignore_patterns('*.beeftv-plugin', '*.tmp', 'node_modules'))
        # The official packager embeds docs; only its archives return to the checkout.
        run(['bash', stage / 'build-packages.sh'], env)
        expected = {path.parent.name + '.beeftv-plugin' for path in stage.glob('*/manifest.json')}
        packages = {path.name: path for path in stage.glob('*.beeftv-plugin')}
        if not expected or packages.keys() != expected:
            raise ValueError('Protocol package generation is incomplete.')
        for path in packages.values():
            with zipfile.ZipFile(path) as archive:
                if archive.testzip() or 'manifest.json' not in archive.namelist():
                    raise ValueError('Invalid protocol package: ' + path.name)
        for name, path in packages.items():
            shutil.copy2(path, source / name)
        for path in source.glob('*.beeftv-plugin'):
            if path.name not in expected:
                path.unlink()


def prepare_wasm_sources(output, version):
    # Reuse upstream's locked sources, checksum rules and collector without changing it.
    collector = runpy.run_path(str(ROOT / 'scripts/package-wasm-sources.py'))
    lock = collector['load_lock'](collector['DEFAULT_LOCK'])
    collector['verify_binary'](ROOT / 'web/node_modules/@ffmpeg/core/dist/esm', lock)
    cache = ROOT / '.local/cache/ffmpeg-wasm-sources'
    cache.mkdir(parents=True, exist_ok=True)
    for index, source in enumerate(lock['sources'], 1):
        print(f"[源码 {index}/{len(lock['sources'])}] {source['id']}：检查缓存或下载……", flush=True)
        for attempt in range(1, 4):
            try:
                collector['source_bytes'](source, cache, False)
                break
            except (IncompleteRead, ConnectionError, TimeoutError, socket.timeout, URLError) as error:
                if isinstance(error, HTTPError) and error.code not in (408, 429, 500, 502, 503, 504):
                    raise
                if isinstance(error, URLError) and isinstance(error.reason, ssl.SSLCertVerificationError):
                    raise
                if attempt == 3:
                    raise ValueError(f"{source['id']} 下载失败（已尝试 3 次）。有效缓存已保留，检查网络后重新运行即可。") from error
                print(f"{source['id']} 下载中断，{attempt * 2} 秒后重试（{attempt + 1}/3）……", flush=True)
                time.sleep(attempt * 2)
    # Each batch gets its own bundle; reruns must not collide with a previous release file.
    print('源码已齐备，正在生成并校验源码包……', flush=True)
    bundle = collector['collect'](lock, version, output, cache, offline=True)
    # Upstream's license index reads this generated input from dist's root.
    distribution = ROOT / 'dist'
    distribution.mkdir(exist_ok=True)
    shutil.copy2(bundle, distribution / bundle.name)
    return bundle


def verify_installer(path, target, env):
    if not path.is_file() or path.stat().st_size == 0:
        raise ValueError('安装包缺失或为空。')
    if target.startswith('darwin/'):
        run(['hdiutil', 'verify', path], env)
    elif target.startswith('linux/'):
        run(['dpkg-deb', '--info', path], env)
    else:
        # NSIS emits a PE installer; this checks structure, not an actual Windows installation.
        with path.open('rb') as stream:
            header = stream.read(64)
            if len(header) != 64 or header[:2] != b'MZ':
                raise ValueError('Windows 安装包不是有效的 PE 文件。')
            stream.seek(int.from_bytes(header[60:64], 'little'))
            if stream.read(4) != b'PE\x00\x00':
                raise ValueError('Windows 安装包 PE 签名无效。')


def build_all(target, env, selected=None):
    version = (ROOT / 'AIHUBKIT_VERSION').read_text().strip()
    if not re.fullmatch(r'v\d+\.\d+\.\d+', version):
        raise ValueError('AIHUBKIT_VERSION 必须是 v主版本.次版本.修订号。')
    entries = brands.registry()
    if selected:
        entries = [entry for entry in entries if entry['id'] == selected]
        if not entries:
            raise ValueError(f'运营清单中不存在品牌：{selected}')
    binaries = ROOT / 'backend/cmd/desktop/build/bin'
    if binaries.is_symlink() or (binaries.exists() and any(binaries.iterdir())):
        raise ValueError('构建目录已有产物，请先移走需保留的产物，或使用干净的 worktree。')
    env = dict(env, GITHUB_REPOSITORY='aihubkit-labs/BeefTV', BEEFTV_RELEASE_TAG='aihubkit-' + version)
    python = python_environment(env, target)
    # Native helpers invoked by upstream scripts must use the same Python environment.
    env['PATH'] = str(python.parent) + os.pathsep + env.get('PATH', '')
    env['PYTHONUTF8'] = '1'
    cache = ROOT / '.local/cache'
    cache.mkdir(parents=True, exist_ok=True)
    target_name = target.replace('/', '-')
    output_root = ROOT / 'dist/aihubkit'
    output_root.mkdir(parents=True, exist_ok=True)
    batch = f'{version}-{target_name}-{datetime.now():%Y%m%d-%H%M%S-%f}'
    output = output_root / (batch + '.partial')
    output.mkdir()
    try:
        with tempfile.TemporaryDirectory(prefix='brand-build-', dir=cache) as directory:
            snapshots = Path(directory) / 'snapshots'
            print('获取品牌信息和图标……', flush=True)
            command = [python, ROOT / 'scripts/aihubkit/brands.py', 'snapshot', '--output', snapshots]
            if selected:
                command += ['--brand', selected]
            run(command, env)
            # Fail before building if the registry changed during snapshot collection.
            frozen = json.loads((snapshots / 'brands.json').read_text(encoding='utf-8'))['brands']
            if [(b['id'], b['origin']) for b in frozen] != [(b['id'], b['origin']) for b in entries]:
                raise ValueError('运营清单已变化，请重新运行构建。')
            shutil.copy2(snapshots / 'brands.json', output / 'brands.json')

            print('准备工具链、依赖和插件……', flush=True)
            prepare_dependencies(env)
            prepare_wasm_sources(output, version)

            for index, entry in enumerate(entries, 1):
                snapshot = snapshots / entry['id']
                values = brands.environment(snapshot)
                brand_env = dict(env, **values)
                print(f"[{index}/{len(entries)}] 构建 {values['BEEFTV_PRODUCT_NAME']}……", flush=True)
                try:
                    run([python, ROOT / 'scripts/aihubkit/brands.py', 'build',
                         '--snapshot', snapshot, '--target', target], brand_env)
                    run([python, ROOT / 'scripts/package-desktop-installer.py',
                         '--platform', target_name, '--output-dir', output], brand_env)
                    # Consume the official packager's filename; do not duplicate its naming rules.
                    name = run([python, '-c',
                         'import sys; '
                         'sys.path.insert(0, "scripts"); '
                         'from runpy import run_path; '
                         'p=run_path("scripts/package-desktop-installer.py"); '
                         'print(p["installer_name"](sys.argv[1],sys.argv[2]))',
                         version, target_name], brand_env, capture=True).stdout.strip()
                    installer = output / name
                    verify_installer(installer, target, brand_env)
                finally:
                    # Preflight refuses existing output, so this tree belongs to this invocation.
                    if binaries.is_dir() and not binaries.is_symlink():
                        shutil.rmtree(binaries)

            components = ['ffprobe-' + target_name] if target.startswith('darwin/') else []
            for component in components:
                name = f'BeefTV-{version}-{component}-sources.tar.gz'
                source = ROOT / 'dist' / name
                if not source.is_file() or source.stat().st_size == 0:
                    raise ValueError('缺少媒体组件源码包：' + name)
                shutil.copy2(source, output / name)
            checksums = []
            for path in sorted(output.iterdir()):
                digest = hashlib.sha256()
                with path.open('rb') as stream:
                    for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                        digest.update(chunk)
                checksums.append(f'{digest.hexdigest()}  {path.name}\n')
            (output / 'SHA256SUMS.txt').write_text(''.join(checksums), encoding='utf-8')
            output = output.rename(output_root / batch)
    except BaseException:
        print(f'打包未完成，部分产物位于：{output}', file=sys.stderr)
        raise
    print(f'全部 {len(entries)} 个品牌打包完成：{output}', flush=True)
    return output


@contextmanager
def checkout_lock(path):
    # Both locks release on process exit, including interrupted builds. Keep the inode stable.
    with path.open('a+b') as lock:
        if lock.tell() == 0:
            lock.write(b'0')
            lock.flush()
        lock.seek(0)
        try:
            if os.name == 'nt':
                import msvcrt
                msvcrt.locking(lock.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            raise ValueError('此工作区已有品牌构建正在运行，请等待完成。') from None
        yield


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--brand', help='仅验收指定品牌；默认打包运营清单中的全部品牌')
    parser.add_argument('--check', action='store_true', help='只检查本机工具链和品牌清单')
    parser.add_argument('--prepare-only', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    if sys.version_info < (3, 9):
        raise ValueError('请使用 Python 3.9 或更新版本。')
    if args.prepare_only:
        prepare_dependencies(os.environ.copy())
        return
    target = native_target()
    entries = brands.registry()
    if args.brand and args.brand not in [entry['id'] for entry in entries]:
        raise ValueError('运营清单中不存在品牌：' + args.brand)
    env = toolchain(target)
    if args.check:
        print(f"工具链检查通过：{target}；品牌：{', '.join(entry['id'] for entry in entries)}")
        return
    local = ROOT / '.local'
    local.mkdir(exist_ok=True)
    # Wails edits shared build files; concurrent brand builds in one checkout would mix identities.
    with checkout_lock(local / 'brand-build.lock'):
        build_all(target, env, args.brand)


if __name__ == '__main__':
    if os.name == 'nt':
        sys.stdout.reconfigure(encoding='utf-8')
        sys.stderr.reconfigure(encoding='utf-8')
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'构建失败：{error}') from error
    except KeyboardInterrupt:
        raise SystemExit('构建已中止。')
