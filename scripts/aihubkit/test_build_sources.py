"""Exercise download recovery with the real upstream checksum and archive collector."""
import hashlib
from http.client import IncompleteRead
import io
import json
from pathlib import Path
import shutil
import socket
import ssl
import tarfile
import tempfile
import unittest
from unittest.mock import Mock, patch
from urllib.error import HTTPError, URLError

import build


class SourcePreparationTests(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix='brand source build ')
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        (self.root / 'scripts').mkdir()
        shutil.copy2(build.ROOT / 'scripts/package-wasm-sources.py', self.root / 'scripts')
        (self.root / 'licenses').mkdir()
        binary = self.root / 'web/node_modules/@ffmpeg/core/dist/esm'
        binary.mkdir(parents=True)
        hashes = {}
        for kind, name in [('wasm', 'ffmpeg-core.wasm'), ('javascript', 'ffmpeg-core.js')]:
            data = kind.encode()
            (binary / name).write_bytes(data)
            hashes[kind] = hashlib.sha256(data).hexdigest()
        data = io.BytesIO()
        with tarfile.open(fileobj=data, mode='w:gz') as archive:
            member = tarfile.TarInfo('fixture/COPYING')
            content = b'Fixture source license\n'
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
        self.data = data.getvalue()
        revision = 'a' * 40
        self.source = dict(id='fixture', repository='test/fixture', revision=revision,
                           filename=f'fixture-{revision}.tar.gz',
                           url=f'https://codeload.github.com/test/fixture/tar.gz/{revision}',
                           sha256=hashlib.sha256(self.data).hexdigest(), licensePaths=['COPYING'])
        self.lock = dict(id='ffmpeg-wasm', version='0.12.10', license='GPL-2.0-or-later',
                         binaryHashes=hashes, provenance={'status': 'unresolved'}, sources=[self.source])
        (self.root / 'licenses/ffmpeg-wasm-sources.json').write_text(json.dumps(self.lock), encoding='utf-8')
        self.cache = self.root / '.local/cache/ffmpeg-wasm-sources' / self.source['filename']
        context = patch.object(build, 'ROOT', self.root)
        context.start()
        self.addCleanup(context.stop)
        sleeper = patch.object(build.time, 'sleep')
        self.sleep = sleeper.start()
        self.addCleanup(sleeper.stop)

    def collect(self, name='batch'):
        return build.prepare_wasm_sources(self.root / name, 'v1.2.3')

    def test_truncated_download_retries_and_reruns_reuse_only_complete_cached_sources(self):
        broken = Mock()
        broken.__enter__ = Mock(return_value=broken)
        broken.__exit__ = Mock(return_value=False)
        broken.read.side_effect = IncompleteRead(b'partial', 99)
        replies = [broken, io.BytesIO(self.data)]

        def respond(*_args, **_kwargs):
            self.assertFalse(self.cache.exists(), 'A partial response must never become a cache hit')
            return replies.pop(0)

        with patch('urllib.request.urlopen', side_effect=respond) as request:
            bundle = self.collect()
        self.assertEqual(request.call_count, 2)
        self.sleep.assert_called_once_with(2)
        self.assertEqual(self.cache.read_bytes(), self.data)
        with patch('urllib.request.urlopen', side_effect=AssertionError('Valid cache must be reused')):
            rerun = self.collect('rerun')
        self.assertEqual(bundle.read_bytes(), rerun.read_bytes())
        self.assertEqual((self.root / 'dist' / bundle.name).read_bytes(), rerun.read_bytes())
        with tarfile.open(rerun) as archive:
            self.assertEqual(archive.extractfile('sources/' + self.source['filename']).read(), self.data)
            self.assertIn('manifest.json', archive.getnames())

    def test_temporary_http_and_connection_failures_stop_after_three_attempts(self):
        for error in [HTTPError(self.source['url'], 503, 'Unavailable', {}, None),
                      TimeoutError('fixture timeout'), socket.timeout('fixture Python 3.9 timeout'),
                      URLError(ConnectionResetError('fixture reset'))]:
            with self.subTest(error=type(error).__name__), patch('urllib.request.urlopen', side_effect=error) as request:
                with self.assertRaisesRegex(ValueError, '已尝试 3 次'):
                    self.collect()
                self.assertEqual(request.call_count, 3)
                self.assertFalse(self.cache.exists())
                self.assertFalse((self.root / 'batch').exists())

    def test_permanent_http_and_certificate_failures_do_not_retry(self):
        for error in [HTTPError(self.source['url'], 404, 'Not found', {}, None),
                      URLError(ssl.SSLCertVerificationError('fixture certificate'))]:
            with self.subTest(error=type(error).__name__), patch('urllib.request.urlopen', side_effect=error) as request:
                with self.assertRaises(URLError):
                    self.collect()
                self.assertEqual(request.call_count, 1)
                self.assertFalse(self.cache.exists())
        self.sleep.assert_not_called()

    def test_checksum_failure_is_not_retried_or_cached(self):
        with patch('urllib.request.urlopen', return_value=io.BytesIO(b'wrong content')) as request:
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                self.collect()
        self.assertEqual(request.call_count, 1)
        self.sleep.assert_not_called()
        self.assertFalse(self.cache.exists())

    def test_invalid_cached_source_is_preserved_and_rejected(self):
        self.cache.parent.mkdir(parents=True)
        self.cache.write_bytes(b'damaged cache')
        with patch('urllib.request.urlopen', side_effect=AssertionError('Must not replace invalid cache')):
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                self.collect()
        self.assertEqual(self.cache.read_bytes(), b'damaged cache')
        self.sleep.assert_not_called()


if __name__ == '__main__':
    unittest.main()
