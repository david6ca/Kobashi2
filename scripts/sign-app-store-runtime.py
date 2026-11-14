"""Sign only staged bundled Mach-O code; never touches an installed app."""
import os,subprocess
from pathlib import Path
root=Path(os.environ['TARGET_BUILD_DIR'])/os.environ['UNLOCALIZED_RESOURCES_FOLDER_PATH']/'Runtime'
subprocess.run(['python3',str(Path(__file__).with_name('prepare-app-store-runtime.py')),'--output',str(root),'--sanitize-only'],check=True)
identity=os.environ.get('EXPANDED_CODE_SIGN_IDENTITY') or '-'
entitlements=Path(os.environ['SRCROOT'])/'Helper.entitlements'
magic={b'\xcf\xfa\xed\xfe',b'\xce\xfa\xed\xfe',b'\xfe\xed\xfa\xcf',b'\xfe\xed\xfa\xce',b'\xca\xfe\xba\xbe',b'\xbe\xba\xfe\xca'}
for path in sorted(root.rglob('*')):
    if not path.is_file() or path.is_symlink():continue
    with path.open('rb') as f:
        if f.read(4) not in magic:continue
    command=['/usr/bin/codesign','--force','--sign',identity]
    if path==root/'python/bin/python3.12':command+=['--entitlements',str(entitlements)]
    subprocess.run(command+[str(path)],check=True)
