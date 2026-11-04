import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name('verify-release-main.sh')


class ReleaseMainTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='aihubkit release ')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.remote = self.root / 'origin.git'
        self.author = self.root / 'author'
        self.work = self.root / 'release checkout'
        self.env = {
            **os.environ,
            'GIT_CONFIG_GLOBAL': os.devnull,
            'GIT_CONFIG_NOSYSTEM': '1',
            'GIT_AUTHOR_NAME': 'Release Test',
            'GIT_AUTHOR_EMAIL': 'release@example.invalid',
            'GIT_COMMITTER_NAME': 'Release Test',
            'GIT_COMMITTER_EMAIL': 'release@example.invalid',
            'GIT_TERMINAL_PROMPT': '0',
        }
        self.git(self.root, 'init', '--bare', '--initial-branch=main', str(self.remote))
        self.git(self.root, 'clone', str(self.remote), str(self.author))
        self.git(self.author, 'commit', '--allow-empty', '-m', 'Initial main')
        self.git(self.author, 'push', 'origin', 'main')
        self.git(self.root, 'clone', str(self.remote), str(self.work))
        self.install_script(self.work)

    def git(self, directory, *args):
        return subprocess.run(['git', '-C', str(directory), *args], env=self.env,
                              capture_output=True, text=True, check=True).stdout.strip()

    def install_script(self, checkout):
        target = checkout / 'scripts/aihubkit/verify-release-main.sh'
        target.parent.mkdir(parents=True)
        shutil.copyfile(SCRIPT, target)

    def verify(self, ref='HEAD', checkout=None):
        checkout = checkout or self.work
        return subprocess.run(['bash', str(checkout / 'scripts/aihubkit/verify-release-main.sh'), ref],
                              cwd=self.root, env=self.env, capture_output=True, text=True, timeout=20)

    def test_accepts_annotated_tag_on_remote_main(self):
        self.git(self.work, 'tag', '-a', 'aihubkit-v1.0.8', '-m', 'Release')
        result = self.verify('refs/tags/aihubkit-v1.0.8')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_unmerged_version_commit_even_when_branch_is_pushed(self):
        self.git(self.work, 'switch', '-c', 'codex/release')
        (self.work / 'AIHUBKIT_VERSION').write_text('v1.0.8\n')
        (self.work / 'AIHUBKIT_CHANGELOG.md').write_text('## v1.0.8\nChanges\n')
        self.git(self.work, 'add', 'AIHUBKIT_VERSION', 'AIHUBKIT_CHANGELOG.md')
        self.git(self.work, 'commit', '-m', 'Prepare release')
        self.git(self.work, 'push', 'origin', 'HEAD')
        self.git(self.work, 'tag', 'aihubkit-v1.0.8')
        result = self.verify('refs/tags/aihubkit-v1.0.8')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('not merged into origin/main', result.stderr)

    def test_refreshes_stale_remote_tracking_branch_after_merge(self):
        self.git(self.author, 'commit', '--allow-empty', '-m', 'Merged release')
        commit = self.git(self.author, 'rev-parse', 'HEAD')
        self.git(self.author, 'push', 'origin', 'main')
        self.assertNotEqual(self.git(self.work, 'rev-parse', 'origin/main'), commit)
        result = self.verify(commit)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_main_advancing_does_not_block_same_release(self):
        commit = self.git(self.work, 'rev-parse', 'HEAD')
        self.git(self.author, 'commit', '--allow-empty', '-m', 'Next change')
        self.git(self.author, 'push', 'origin', 'main')
        result = self.verify(commit)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.git(self.work, 'rev-parse', 'origin/main'), commit)

    def test_unavailable_remote_cannot_use_stale_local_reference(self):
        self.git(self.work, 'remote', 'set-url', 'origin', str(self.root / 'missing.git'))
        result = self.verify()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Cannot refresh origin/main', result.stderr)

    def test_missing_remote_main_is_rejected(self):
        self.git(self.remote, 'update-ref', '-d', 'refs/heads/main')
        result = self.verify()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Cannot refresh origin/main', result.stderr)

    def test_shallow_checkout_fetches_history_before_checking(self):
        self.git(self.author, 'tag', 'aihubkit-v1.0.8')
        self.git(self.author, 'commit', '--allow-empty', '-m', 'Next change')
        self.git(self.author, 'push', 'origin', 'main', 'refs/tags/aihubkit-v1.0.8')
        checkout = self.root / 'shallow'
        self.git(self.root, 'clone', '--depth=1', '--branch=aihubkit-v1.0.8',
                 self.remote.as_uri(), str(checkout))
        self.install_script(checkout)
        result = self.verify('refs/tags/aihubkit-v1.0.8', checkout)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.git(checkout, 'rev-parse', '--is-shallow-repository'), 'false')


if __name__ == '__main__':
    unittest.main()
