import hashlib
from pathlib import Path
import tempfile
import unittest
from verify_uploaded_assets import reconcile


class UploadedAssetsTests(unittest.TestCase):
    def test_normalized_name_preserves_checksum_and_display_name(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / '客户 TV.zip').write_bytes(b'fixture')
            digest = hashlib.sha256(b'fixture').hexdigest()
            asset = dict(name='TV.zip', digest='sha256:' + digest, size=7, state='uploaded')
            notes = reconcile(root, [asset], '客户 TV: `客户 TV.zip`')
            self.assertEqual(notes, '客户 TV: `TV.zip`')
            self.assertEqual((root / 'SHA256SUMS.txt').read_text(), digest + '  TV.zip\n')

    def test_missing_corrupted_or_incomplete_upload_blocks_publication(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'client.zip').write_bytes(b'fixture')
            asset = dict(name='client.zip', digest='sha256:' + hashlib.sha256(b'fixture').hexdigest(), size=7, state='uploaded')
            for assets in [[], [dict(asset, digest='sha256:wrong')], [dict(asset, state='starter')], [dict(asset, size=8)]]:
                with self.subTest(assets=assets), self.assertRaises(ValueError):
                    reconcile(root, assets, '')


if __name__ == '__main__':
    unittest.main()
