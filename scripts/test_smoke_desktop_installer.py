import errno
import importlib.util
from pathlib import Path
import sys
import urllib.error
import unittest
from unittest.mock import MagicMock, patch

spec = importlib.util.spec_from_file_location('installer_smoke', Path(__file__).with_name('smoke-desktop-installer.py'))
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class WebViewCloseTests(unittest.TestCase):
    def test_timeout_keeps_waiting_until_connection_is_refused(self):
        process = MagicMock(spec=['webview_port', 'pid', 'poll', 'terminate', 'kill', 'wait'])
        process.webview_port = 12345
        opener = MagicMock()
        opener.open.side_effect = [urllib.error.URLError(TimeoutError('slow endpoint')),
                                   urllib.error.URLError(ConnectionRefusedError(errno.ECONNREFUSED, 'closed'))]
        with patch.object(smoke.urllib.request, 'build_opener', return_value=opener), patch.object(smoke.time, 'sleep'):
            smoke.stop(process)
        self.assertEqual(opener.open.call_count, 2)

    def test_only_timeouts_fail_instead_of_crediting_an_old_webview(self):
        process = MagicMock(spec=['webview_port', 'pid', 'poll', 'terminate', 'kill', 'wait'])
        process.webview_port = 12345
        opener = MagicMock()
        opener.open.side_effect = urllib.error.URLError(TimeoutError('still listening'))
        with patch.object(smoke.urllib.request, 'build_opener', return_value=opener), patch.object(smoke.time, 'sleep'), patch.object(smoke.time, 'monotonic', side_effect=[0, 1, 31]):
            with self.assertRaisesRegex(RuntimeError, 'Old WebView remained running'):
                smoke.stop(process)


class WindowsInstallerContractTests(unittest.TestCase):
    def test_packager_compiles_windows_installer_as_utf8_without_warnings(self):
        import os
        import shutil
        import subprocess
        import tempfile
        packager_path = Path(__file__).with_name('package-desktop-installer.py')
        if not shutil.which('makensis'):
            self.skipTest('NSIS compiler not installed')
        for install, name in [('BeefTV', 'BeefTV'), ('fixtureTV', '客户平台 TV')]:
          with self.subTest(brand=install), tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            payload = root / 'payload'
            payload.mkdir()
            (payload / (install + '.exe')).write_bytes(b'fixture')
            out_dir = root / 'dist'
            result = subprocess.run(
                ['python3', str(packager_path), '--platform', 'windows-amd64',
                 '--input', str(payload), '--output-dir', str(out_dir)],
                env=dict(os.environ, BEEFTV_INSTALL_NAME=install, BEEFTV_PRODUCT_NAME=name, BEEFTV_ARTIFACT_NAME=name, BEEFTV_PACKAGE_ID=install.lower()),
                capture_output=True, text=True, check=True,
            )
            self.assertIn('(UTF8)', result.stdout)
            self.assertNotIn('warning', result.stdout.lower())
            self.assertEqual(len(list(out_dir.glob(name + '-*-windows-amd64-setup.exe'))), 1)


@unittest.skipUnless(sys.platform == 'darwin', 'native macOS disk image verification')
class MacDiskImageTests(unittest.TestCase):
    def test_brand_disk_image_and_app_share_full_system_name(self):
        import os
        import plistlib
        import shutil
        import subprocess
        import tempfile
        name = '客户平台-Token源头New APITV'
        with tempfile.TemporaryDirectory(prefix='brand-native-dmg-') as directory:
            root = Path(directory)
            payload = root / (name + '.app')
            binary = payload / 'Contents/MacOS/BeefTV'
            binary.parent.mkdir(parents=True)
            shutil.copyfile('/usr/bin/true', binary)
            binary.chmod(0o755)
            with (payload / 'Contents/Info.plist').open('wb') as info:
                plistlib.dump({'CFBundleExecutable': 'BeefTV', 'CFBundleIdentifier': 'test.brand.dmg',
                               'CFBundlePackageType': 'APPL', 'CFBundleVersion': '1',
                               'CFBundleName': name, 'CFBundleDisplayName': name}, info)
            subprocess.run(['codesign', '--force', '--sign', '-', str(payload)], check=True, capture_output=True)
            subprocess.run([sys.executable, str(Path(__file__).with_name('package-desktop-installer.py')),
                            '--platform', 'darwin-arm64', '--input', str(payload), '--output-dir', str(root), '--version', 'v1.0.0'],
                           env=dict(os.environ, BEEFTV_INSTALL_NAME='fixtureTV', BEEFTV_PACKAGE_ID='fixturetv',
                                    BEEFTV_PRODUCT_NAME=name, BEEFTV_ARTIFACT_NAME=name),
                           check=True, capture_output=True)
            installer = root / (name + '-v1.0.0-darwin-arm64.dmg')
            subprocess.run(['hdiutil', 'verify', str(installer)], check=True, capture_output=True)
            mount = root / 'mounted'
            subprocess.run(['hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(installer)], check=True, capture_output=True)
            try:
                self.assertTrue((mount / (name + '.app')).is_dir())
                self.assertFalse((mount / 'fixtureTV.app').exists())
                info = plistlib.loads(subprocess.check_output(['diskutil', 'info', '-plist', str(mount)]))
                self.assertEqual(info['VolumeName'], name)
                with (mount / (name + '.app') / 'Contents/Info.plist').open('rb') as stream:
                    self.assertEqual(plistlib.load(stream)['CFBundleDisplayName'], name)
            finally:
                subprocess.run(['hdiutil', 'detach', str(mount)], check=True, capture_output=True)

    def test_real_packager_creates_verified_hfs_image(self):
        import plistlib
        import shutil
        import subprocess
        import tempfile
        packager_spec = importlib.util.spec_from_file_location(
            'installer_packager', Path(__file__).with_name('package-desktop-installer.py'))
        packager = importlib.util.module_from_spec(packager_spec)
        packager_spec.loader.exec_module(packager)
        with tempfile.TemporaryDirectory(prefix='beeftv-native-dmg-') as directory:
            root = Path(directory)
            payload = root / 'fixture with spaces' / 'BeefTV.app'
            binary = payload / 'Contents/MacOS/BeefTV'
            binary.parent.mkdir(parents=True)
            shutil.copyfile('/usr/bin/true', binary)
            binary.chmod(0o755)
            with (payload / 'Contents/Info.plist').open('wb') as info:
                plistlib.dump({'CFBundleExecutable': 'BeefTV', 'CFBundleIdentifier': 'test.beeftv.dmg',
                               'CFBundlePackageType': 'APPL', 'CFBundleVersion': '1'}, info)
            subprocess.run(['codesign', '--force', '--sign', '-', str(payload)], check=True,
                           capture_output=True, text=True)
            output = root / 'fixture.dmg'
            packager.package('v1.0.0', 'darwin-amd64', payload, output)
            image_info = subprocess.run(['hdiutil', 'imageinfo', str(output)], check=True,
                                        capture_output=True, text=True).stdout
            self.assertIn('Apple_HFS', image_info)
            self.assertNotIn('Apple_APFS', image_info)
            subprocess.run(['hdiutil', 'verify', str(output)], check=True,
                           capture_output=True, text=True)


if __name__ == '__main__':
    unittest.main()
