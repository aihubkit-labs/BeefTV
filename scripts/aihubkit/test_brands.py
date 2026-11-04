import base64
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))
import brands


class BrandTests(unittest.TestCase):
    def setUp(self):
        self.entry = {'id': 'fixture', 'origin': 'https://brand.example'}
        self.data = dict(system_name='客户平台', logo='https://cdn.example/logo.png',
                         server_address='https://site.example', wechat_qrcode='https://cdn.example/qr.png',
                         api_server_address='https://api.example')

    def test_domain_and_api_are_distinct_and_name_comes_from_status(self):
        value = brands.metadata(self.entry, {'success': True, 'data': self.data})
        self.assertEqual(value['origin'], 'https://brand.example')
        self.assertEqual(value['apiOrigin'], 'https://api.example')
        self.assertEqual(value['appName'], '客户平台TV')
        self.assertEqual(value['artifactName'], '客户平台TV')

    def test_invalid_status_does_not_fall_back_to_a_default_brand(self):
        for data in [dict(self.data, system_name=''), dict(self.data, api_server_address=''), dict(self.data, logo='http://cdn.example/icon'), dict(self.data, api_server_address='https://name:pass@api.example')]:
            with self.subTest(data=data), self.assertRaises(ValueError):
                brands.metadata(self.entry, {'success': True, 'data': data})

    def test_registry_rejects_duplicate_ids_or_origins(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'brands.json'
            for entries in [[self.entry, self.entry], [self.entry, dict(self.entry, id='second')], []]:
                path.write_text(json.dumps({'brands': entries}))
                with self.assertRaises(ValueError):
                    brands.registry(path)

    def test_snapshot_embeds_one_brand_and_pins_logo_bytes(self):
        from PIL import Image
        import io
        image = io.BytesIO()
        Image.new('RGBA', (64, 64), (20, 40, 60, 255)).save(image, format='PNG')
        with tempfile.TemporaryDirectory() as directory, patch.object(brands, 'registry', return_value=[self.entry]), patch.object(brands, 'download', side_effect=[json.dumps({'success': True, 'data': self.data}).encode(), image.getvalue()]):
            root = Path(directory)
            brands.snapshot(root)
            env = brands.environment(root / 'fixture')
            self.assertEqual(env['BEEFTV_INSTALL_NAME'], 'fixtureTV')
            self.assertEqual(env['BEEFTV_PRODUCT_NAME'], '客户平台TV')
            encoded = env['BEEFTV_EXTRA_LDFLAGS'].split('branding.Snapshot=')[1].split()[0]
            self.assertEqual(json.loads(base64.b64decode(encoded))['apiOrigin'], 'https://api.example')
            (root / 'fixture/logo.png').write_bytes(b'wrong icon')
            with self.assertRaisesRegex(ValueError, 'checksum'):
                brands.load_snapshot(root / 'fixture')


if __name__ == '__main__':
    unittest.main()
