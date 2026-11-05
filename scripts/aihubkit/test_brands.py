import base64
from html.parser import HTMLParser
import importlib.util
import json
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent))
import brands


class HeadLinks(HTMLParser):
    def __init__(self, html):
        super().__init__()
        self.links = []
        self.in_head = False
        self.feed(html)

    def handle_starttag(self, tag, attrs):
        if tag == 'head':
            self.in_head = True
        if tag == 'link' and self.in_head:
            self.links.append(dict(attrs))

    def handle_endtag(self, tag):
        if tag == 'head':
            self.in_head = False


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


class BrandBuildTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.config = self.root / 'backend/cmd/desktop/wails.json'
        self.config.parent.mkdir(parents=True)
        self.config.write_bytes(b'{"info": {}, "frontend:build": "bun run build:desktop"}\r\n')
        self.index = self.root / 'web/index.html'
        self.index.parent.mkdir()
        self.index.write_bytes(b'<!doctype html>\r\n<html><HEAD><title>Fixture</title></HEAD><body><div id="root"></div></body></html>')
        self.public = self.root / 'web/public/aihubkit-brand'
        self.public.parent.mkdir()
        self.css = self.root / 'web/src/integrations/aihubkit/brand-loading.css'
        self.css.parent.mkdir(parents=True)
        self.css.write_bytes((brands.ROOT / 'web/src/integrations/aihubkit/brand-loading.css').read_bytes())
        self.snapshot = self.root / 'snapshot'
        self.snapshot.mkdir()
        self.originals = {path: path.read_bytes() for path in [self.index, self.config]}
        root_patch = patch.object(brands, 'ROOT', self.root)
        root_patch.start()
        self.addCleanup(root_patch.stop)
        env_patch = patch.object(brands, 'environment', return_value={
            'BEEFTV_PRODUCT_NAME': 'Fixture TV', 'BEEFTV_PACKAGE_ID': 'fixturetv', 'BEEFTV_INSTALL_NAME': 'fixtureTV',
        })
        env_patch.start()
        self.addCleanup(env_patch.stop)

    def check_restored(self):
        for path, data in self.originals.items():
            self.assertEqual(path.read_bytes(), data)
        self.assertFalse(self.public.exists())

    def check_build_inputs(self, *_args, **_kwargs):
        links = HeadLinks(self.index.read_text()).links
        self.assertIn(dict(rel='stylesheet', href='/aihubkit-brand/loading.css'), links)
        self.assertIn(dict(rel='preload', **{'as': 'image'}, href='/aihubkit-brand/logo.png'), links)
        self.assertEqual((self.public / 'logo.png').read_bytes(), (self.snapshot / 'logo.png').read_bytes())
        self.assertEqual((self.public / 'loading.css').read_bytes(), self.css.read_bytes())
        self.assertEqual(json.loads(self.config.read_text())['frontend:build'], 'bun run build:desktop')

    def test_each_brand_gets_its_own_initial_logo_and_upstream_files_are_restored(self):
        for logo in [b'first-brand', b'second-brand']:
            (self.snapshot / 'logo.png').write_bytes(logo)
            with patch.object(brands.subprocess, 'run', side_effect=self.check_build_inputs):
                brands.build(self.snapshot, 'windows/amd64')
            self.check_restored()

    def test_failed_build_restores_html_config_and_removes_brand_assets(self):
        (self.snapshot / 'logo.png').write_bytes(b'brand')
        def fail(*args, **kwargs):
            self.check_build_inputs()
            raise subprocess.CalledProcessError(1, args[0])
        with patch.object(brands.subprocess, 'run', side_effect=fail):
            with self.assertRaises(subprocess.CalledProcessError):
                brands.build(self.snapshot, 'windows/amd64')
        self.check_restored()

    def test_missing_stylesheet_stops_before_build_and_restores_inputs(self):
        (self.snapshot / 'logo.png').write_bytes(b'brand')
        self.css.unlink()
        with patch.object(brands.subprocess, 'run') as runner:
            with self.assertRaises(FileNotFoundError):
                brands.build(self.snapshot, 'windows/amd64')
            runner.assert_not_called()
        self.check_restored()

    def test_unexpected_html_fails_without_building_or_creating_brand_assets(self):
        self.index.write_bytes(b'<html><body>no head</body></html>')
        self.originals[self.index] = self.index.read_bytes()
        with patch.object(brands.subprocess, 'run') as runner:
            with self.assertRaisesRegex(ValueError, 'head element'):
                brands.build(self.snapshot, 'windows/amd64')
            runner.assert_not_called()
        self.check_restored()


if __name__ == '__main__':
    unittest.main()
