import hashlib
from pathlib import Path
import tempfile
import unittest
from verify_uploaded_assets import reconcile


class UploadedAssetsTests(unittest.TestCase):
    def test_matching_upload_preserves_checksum_and_release_notes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'CustomerTV.zip').write_bytes(b'fixture')
            digest = hashlib.sha256(b'fixture').hexdigest()
            asset = dict(name='CustomerTV.zip', digest='sha256:' + digest, size=7, state='uploaded')
            notes = reconcile(root, [asset], 'CustomerTV: `CustomerTV.zip`')
            self.assertEqual(notes, 'CustomerTV: `CustomerTV.zip`')
            self.assertEqual((root / 'SHA256SUMS.txt').read_text(), digest + '  CustomerTV.zip\n')

    def test_normalized_name_blocks_publication_even_when_bytes_match(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / '客户 TV.zip').write_bytes(b'fixture')
            asset = dict(name='TV.zip', digest='sha256:' + hashlib.sha256(b'fixture').hexdigest(), size=7, state='uploaded')
            with self.assertRaisesRegex(ValueError, 'filenames differ'):
                reconcile(root, [asset], '客户 TV: `客户 TV.zip`')
            self.assertFalse((root / 'SHA256SUMS.txt').exists())

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
