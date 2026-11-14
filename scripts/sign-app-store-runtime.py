"""Sign only the staged native engine; never touches an installed app."""
import os,subprocess
from pathlib import Path
root=Path(os.environ['TARGET_BUILD_DIR'])/os.environ['UNLOCALIZED_RESOURCES_FOLDER_PATH']/'Runtime'
identity=os.environ.get('EXPANDED_CODE_SIGN_IDENTITY') or '-'
entitlements=Path(os.environ['SRCROOT'])/'Helper.entitlements'
for path in sorted(root.iterdir()):
    if path.is_symlink() or not path.is_file() or (path.suffix!='.dylib' and path.name!='llama-server'):continue
    command=['/usr/bin/codesign','--force','--sign',identity]
    if path.name=='llama-server':command+=['--entitlements',str(entitlements)]
    subprocess.run(command+[str(path)],check=True)
