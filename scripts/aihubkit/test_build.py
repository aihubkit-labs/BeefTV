import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import brands
import build


class NativeBuildTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix='brand build with spaces ')
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.binaries = self.root / 'backend/cmd/desktop/build/bin'
        self.binaries.mkdir(parents=True)
        (self.root / 'AIHUBKIT_VERSION').write_text('v1.0.9\n')
        self.entries = [{'id': key, 'origin': f'https://{key}.example'} for key in ('first', 'second')]
        self.calls = []
        self.fail = None
        self.target = 'darwin/arm64'
        for context in (
            patch.object(build, 'ROOT', self.root),
            patch.object(brands, 'ROOT', self.root),
            patch.object(brands, 'registry', return_value=self.entries),
            patch.object(build, 'python_environment', return_value=Path(sys.executable)),
            patch.object(build, 'prepare_dependencies', side_effect=lambda env: self.calls.append(('prepare', env))),
            patch.object(build, 'prepare_wasm_sources', side_effect=self.prepare_sources),
            patch.object(build, 'run', side_effect=self.execute),
        ):
            context.start()
            self.addCleanup(context.stop)

    def prepare_sources(self, output, version):
        self.calls.append(('sources', {}))
        if self.fail == 'sources':
            raise ValueError('fixture download failure')
        path = output / f'BeefTV-{version}-ffmpeg-wasm-sources.tar.gz'
        path.write_bytes(b'wasm-sources')
        return path

    def filename(self, env):
        suffix = {'darwin/arm64': '.dmg', 'darwin/amd64': '.dmg',
                  'windows/amd64': '-setup.exe', 'linux/amd64': '.deb'}[self.target]
        return env['BEEFTV_ARTIFACT_NAME'] + '-v1.0.9-' + self.target.replace('/', '-') + suffix

    def execute(self, args, env, cwd=None, capture=False):
        args = [str(arg) for arg in args]
        step = args[0] if args[0] in ('hdiutil', 'dpkg-deb') else Path(args[1]).name
        if step == 'brands.py':
            step = args[2]
        self.calls.append((step, env.copy()))
        if step == self.fail:
            raise subprocess.CalledProcessError(1, args)
        if step == 'snapshot':
            directory = Path(args[args.index('--output') + 1])
            entries = self.entries
            if '--brand' in args:
                entries = [b for b in entries if b['id'] == args[args.index('--brand') + 1]]
            resolved = []
            for entry in entries:
                brand = brands.metadata(entry, {'success': True, 'data': {
                    'system_name': '客户 空格 ' + entry['id'], 'logo': 'https://cdn.example/icon.png',
                    'server_address': entry['origin'], 'api_server_address': 'https://api.example',
                }})
                folder = directory / entry['id']
                folder.mkdir(parents=True)
                (folder / 'logo.png').write_bytes(b'fixture-logo')
                brand['logoSHA256'] = hashlib.sha256(b'fixture-logo').hexdigest()
                (folder / 'brand.json').write_text(json.dumps(brand), encoding='utf-8')
                resolved.append(brand)
            (directory / 'brands.json').write_text(json.dumps({'brands': resolved}), encoding='utf-8')
        elif step == 'build':
            self.assertEqual(args[args.index('--target') + 1], self.target)
            self.binaries.mkdir(parents=True, exist_ok=True)
            (self.binaries / env['BEEFTV_INSTALL_NAME']).write_bytes(b'fixture-app')
            if self.target.startswith('darwin/'):
                name = 'BeefTV-v1.0.9-ffprobe-' + self.target.replace('/', '-') + '-sources.tar.gz'
                (self.root / 'dist' / name).write_bytes(b'native-sources')
        elif step == 'package-desktop-installer.py':
            self.assertEqual(args[args.index('--platform') + 1], self.target.replace('/', '-'))
            output = Path(args[args.index('--output-dir') + 1])
            content = env['BEEFTV_PRODUCT_NAME'].encode()
            if self.target.startswith('windows/'):
                content = b'MZ' + bytes(58) + (64).to_bytes(4, 'little') + b'PE\x00\x00' + content
            (output / self.filename(env)).write_bytes(content)
        elif step == '-c':
            return subprocess.CompletedProcess(args, 0, self.filename(env) + '\n')
        return subprocess.CompletedProcess(args, 0, '')

    def test_all_platforms_build_every_brand_with_matching_identity_and_checksums(self):
        for target in ('darwin/arm64', 'darwin/amd64', 'windows/amd64', 'linux/amd64'):
            with self.subTest(target=target):
                self.target = target
                self.calls.clear()
                output = build.build_all(target, {})
                self.assertFalse(output.name.endswith('.partial'))
                builds = [env for step, env in self.calls if step == 'build']
                packages = [env for step, env in self.calls if step == 'package-desktop-installer.py']
                self.assertEqual(builds, packages)
                self.assertEqual([env['BEEFTV_INSTALL_NAME'] for env in builds], ['firstTV', 'secondTV'])
                for env in builds:
                    self.assertTrue((output / self.filename(env)).is_file())
                    self.assertEqual(env['PYTHONUTF8'], '1')
                self.assertEqual([step for step, _ in self.calls].count('prepare'), 1)
                expected_sources = 2 if target.startswith('darwin/') else 1
                self.assertEqual(len(list(output.glob('*-sources.tar.gz'))), expected_sources)
                checksum_files = []
                for line in (output / 'SHA256SUMS.txt').read_text(encoding='utf-8').splitlines():
                    digest, name = line.split('  ', 1)
                    self.assertEqual(digest, hashlib.sha256((output / name).read_bytes()).hexdigest())
                    checksum_files.append(name)
                self.assertEqual(set(checksum_files), {path.name for path in output.iterdir()} - {'SHA256SUMS.txt'})
                self.assertFalse(self.binaries.exists())
                self.assertFalse(list((self.root / '.local/cache').iterdir()))
                self.assertEqual([step for step, _ in self.calls].count('hdiutil'), 2 if target.startswith('darwin/') else 0)
                self.assertEqual([step for step, _ in self.calls].count('dpkg-deb'), 2 if target.startswith('linux/') else 0)

    def test_verification_failure_stops_remaining_brands_and_keeps_output_partial(self):
        self.fail = 'hdiutil'
        with self.assertRaises(subprocess.CalledProcessError):
            build.build_all(self.target, {})
        self.assertEqual([step for step, _ in self.calls].count('build'), 1)
        output, = (self.root / 'dist/aihubkit').iterdir()
        self.assertTrue(output.name.endswith('.partial'))
        self.assertFalse((output / 'SHA256SUMS.txt').exists())
        self.assertFalse(self.binaries.exists())

    def test_snapshot_failure_does_not_prepare_or_build(self):
        self.fail = 'snapshot'
        with self.assertRaises(subprocess.CalledProcessError):
            build.build_all(self.target, {})
        self.assertEqual([step for step, _ in self.calls], ['snapshot'])

    def test_source_failure_stops_before_building_and_preserves_partial_output(self):
        self.fail = 'sources'
        with self.assertRaisesRegex(ValueError, 'download failure'):
            build.build_all(self.target, {})
        self.assertEqual([step for step, _ in self.calls], ['snapshot', 'prepare', 'sources'])
        output, = (self.root / 'dist/aihubkit').iterdir()
        self.assertTrue(output.name.endswith('.partial'))
        self.assertFalse((output / 'SHA256SUMS.txt').exists())

    def test_existing_build_is_preserved_before_any_work(self):
        (self.binaries / 'keep').write_text('existing')
        with self.assertRaisesRegex(ValueError, '已有产物'):
            build.build_all(self.target, {})
        self.assertEqual((self.binaries / 'keep').read_text(), 'existing')
        self.assertEqual(self.calls, [])

    def test_optional_single_brand_and_unknown_brand(self):
        with self.assertRaisesRegex(ValueError, '不存在品牌'):
            build.build_all(self.target, {}, 'missing')
        output = build.build_all(self.target, {}, 'second')
        self.assertEqual(len(list(output.glob('*.dmg'))), 1)
        frozen = json.loads((output / 'brands.json').read_text())['brands']
        self.assertEqual([brand['id'] for brand in frozen], ['second'])


class HostTests(unittest.TestCase):
    def test_windows_tools_and_node_runtime_reach_the_official_build_checks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'backend').mkdir()
            (root / 'backend/go.mod').write_text('module fixture\n\ngo 1.26.0\n')
            compiler = root / 'msys64/ucrt64'
            runtime = root / 'node runtime'
            configured = {'PATH': 'original-tools', 'MSYSTEM_PREFIX': str(compiler),
                          'BEEFTV_NODE_RUNTIME': str(runtime), 'BEEFTV_GO_DIR': str(root / 'go'),
                          'BEEFTV_SKIP_LOCAL_VERIFY': '1'}
            def locate(name, **_kwargs):
                return None if name == 'pwsh' else str(root / 'tools' / (name + '.exe'))
            with patch.object(build, 'ROOT', root), patch.dict(os.environ, configured, clear=True), \
                    patch.object(build.shutil, 'which', side_effect=locate), \
                    patch.object(build, 'run', return_value=subprocess.CompletedProcess([], 0, '24.22.1\n')) as runner:
                env = build.toolchain('windows/amd64')
            self.assertEqual(env['CC'], str(compiler / 'bin/gcc.exe'))
            self.assertEqual(env['PATH'].split(os.pathsep)[:1], [str(runtime)])
            for folder in [compiler / 'bin', compiler.parent / 'usr/bin']:
                self.assertIn(str(folder), env['PATH'])
            self.assertEqual(env['BEEFTV_NODE_RUNTIME'], str(runtime))
            self.assertEqual(env['BEEFTV_WAILS_PLATFORM'], 'windows/amd64')
            self.assertEqual(env['CGO_ENABLED'], '1')
            self.assertEqual(env['BEEFTV_SKIP_LOCAL_VERIFY'], '0')
            checks = [call.args[0] for call in runner.call_args_list]
            self.assertIn(root / 'scripts/assert-windows-dwarf5-cgo.ps1', checks[1])
            self.assertEqual(checks[2], ['bun', 'scripts/package-agent-host.mjs', 'windows/amd64', '--verify-runtime'])

    def test_child_executable_uses_prepared_path_and_windows_powershell_environment(self):
        with patch.object(build.shutil, 'which', return_value='/prepared tools/powershell.exe'), \
                patch.object(build.subprocess, 'run') as runner:
            env = {'PATH': '/prepared tools', 'PSModulePath': 'pwsh modules'}
            build.run(['powershell', '-File', Path('file with spaces.ps1')], env)
        self.assertEqual(runner.call_args.args[0], ['/prepared tools/powershell.exe', '-File', 'file with spaces.ps1'])
        self.assertNotIn('PSModulePath', runner.call_args.kwargs['env'])
        self.assertEqual(env['PSModulePath'], 'pwsh modules')

    def test_supported_host_architectures_and_explicitly_rejected_targets(self):
        for system, machine, target in (
            ('Darwin', 'arm64', 'darwin/arm64'), ('Darwin', 'x86_64', 'darwin/amd64'),
            ('Windows', 'AMD64', 'windows/amd64'), ('Linux', 'x86_64', 'linux/amd64'),
            ('Windows', 'ARM64', None), ('Linux', 'aarch64', None), ('Windows', 'x86', None),
        ):
            with self.subTest(system=system, machine=machine), \
                    patch.object(build.platform, 'system', return_value=system), \
                    patch.object(build.platform, 'machine', return_value=machine), \
                    patch.dict(os.environ, {}, clear=True), \
                    patch.object(build.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, '0\n')):
                if target:
                    self.assertEqual(build.native_target(), target)
                else:
                    with self.assertRaisesRegex(ValueError, '暂不支持'):
                        build.native_target()

    def test_windows_arm_host_is_not_mistaken_for_x64_under_emulation(self):
        with patch.object(build.platform, 'system', return_value='Windows'), \
                patch.object(build.platform, 'machine', return_value='AMD64'), \
                patch.dict(os.environ, {'PROCESSOR_ARCHITEW6432': 'ARM64'}):
            with self.assertRaisesRegex(ValueError, '暂不支持'):
                build.native_target()

    def test_rosetta_interpreter_restarts_with_native_python_before_selecting_tools(self):
        with patch.object(sys, 'argv', ['build.py', '--check']), \
                patch.object(build.platform, 'system', return_value='Darwin'), \
                patch.object(build.platform, 'machine', return_value='x86_64'), \
                patch.object(build.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, '1\n')), \
                patch.object(build.os, 'execv', side_effect=SystemExit(0)) as restart:
            with self.assertRaises(SystemExit):
                build.native_target()
            command, args = restart.call_args.args
            self.assertEqual(command, '/usr/bin/arch')
            self.assertEqual(args[:3], ['arch', '-arm64', '/usr/bin/python3'])
            self.assertEqual(args[-1], '--check')

    def test_python_venv_paths_match_each_system(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(build, 'ROOT', Path(directory)), \
                patch.object(build, 'run') as runner:
            for target, location in [('windows/amd64', 'Scripts/python.exe'), ('linux/amd64', 'bin/python3'), ('darwin/arm64', 'bin/python3')]:
                with self.subTest(target=target):
                    result = build.python_environment({}, target)
                    self.assertEqual(result, Path(directory) / '.local/brand-build-venv' / location)
                    self.assertEqual(runner.call_args.args[0][0], result)

    def test_corrupt_windows_installer_cannot_complete_a_batch(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'setup.exe'
            for content in [b'not an executable', b'MZ' + bytes(62)]:
                path.write_bytes(content)
                with self.assertRaises(ValueError):
                    build.verify_installer(path, 'windows/amd64', {})

    def test_checkout_lock_excludes_other_process_and_releases_after_exception(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'build.lock'
            code = 'import sys; from pathlib import Path; import build; build.checkout_lock(Path(sys.argv[1])).__enter__()'
            command = [sys.executable, '-c', code, str(path)]
            cwd = Path(__file__).parent
            with self.assertRaises(RuntimeError):
                with build.checkout_lock(path):
                    result = subprocess.run(command, cwd=cwd, text=True, capture_output=True)
                    self.assertNotEqual(result.returncode, 0)
                    raise RuntimeError('fixture')
            result = subprocess.run(command, cwd=cwd, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == '__main__':
    unittest.main()
