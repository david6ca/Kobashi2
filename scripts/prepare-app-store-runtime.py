"""Copy a verified staged runtime into an app-local, relocatable resource tree."""
import argparse,json,shutil,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--venv',required=True);p.add_argument('--output',required=True);a=p.parse_args()
venv=Path(a.venv).resolve();destination=Path(a.output)
python=(venv/'bin/python3').resolve();base=python.parent.parent
if destination.exists():raise SystemExit('Choose an empty runtime destination')
destination.mkdir(parents=True)
shutil.copytree(base,destination/'python',symlinks=True,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
site=venv/'lib/python3.12/site-packages'
shutil.copytree(site,destination/'site-packages',symlinks=True,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
# Runtime imports must not depend on build-machine paths or installer-only .pth hooks.
for path in (destination/'site-packages').glob('*.pth'):
    lines=[line for line in path.read_text().splitlines() if not line.startswith('/')]
    path.write_text('\n'.join(lines)+'\n')
root=Path(__file__).resolve().parents[1]
shutil.copyfile(root/'internal/bridge/local_support.py',destination/'support.py')
(destination/'site-packages/sitecustomize.py').write_text('import onnxruntime\nonnxruntime.disable_telemetry_events()\n')
packages=[]
for directory in sorted(site.glob('*.dist-info')):
    metadata=directory/'METADATA'
    text=metadata.read_text(errors='replace') if metadata.exists() else ''
    fields=dict(line.split(': ',1) for line in text.splitlines() if line.startswith(('Name: ','Version: ')))
    packages.append(fields)
(destination/'runtime-manifest.json').write_text(json.dumps({'python':'3.12.12','omlx':'0.7.3','packages':packages},indent=2)+'\n')
# Keep every symlink app-relative. Standalone CPython's internal aliases are valid.
for path in destination.rglob('*'):
    if path.is_symlink() and str(path.readlink()).startswith('/'):
        raise SystemExit('Runtime contains an absolute symlink: '+str(path.relative_to(destination)))
print(destination)
