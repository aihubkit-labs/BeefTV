"""Optional distribution identity shared by installer packaging and validation."""
import os
import re

INSTALL_NAME = os.environ.get('BEEFTV_INSTALL_NAME', 'BeefTV')
PACKAGE_ID = os.environ.get('BEEFTV_PACKAGE_ID', 'beeftv')
PRODUCT_NAME = os.environ.get('BEEFTV_PRODUCT_NAME', 'BeefTV')
ARTIFACT_NAME = os.environ.get('BEEFTV_ARTIFACT_NAME', 'BeefTV')
if not re.fullmatch(r'[A-Za-z][A-Za-z0-9-]{0,40}', INSTALL_NAME):
    raise ValueError('Invalid distribution install name')
if not re.fullmatch(r'[a-z][a-z0-9-]{0,40}', PACKAGE_ID):
    raise ValueError('Invalid distribution package ID')
if not ARTIFACT_NAME or any(c in ARTIFACT_NAME for c in '<>:"/\\|?*\r\n'):
    raise ValueError('Invalid distribution artifact name')
if not PRODUCT_NAME or any(ord(c) < 32 or c in '<>:"/\\|?*' for c in PRODUCT_NAME):
    raise ValueError('Invalid distribution product name')
MAC_BUNDLE = PRODUCT_NAME + '.app'
LINUX_BUNDLE = INSTALL_NAME + '-linux'
WINDOWS_EXE = INSTALL_NAME + '.exe'
