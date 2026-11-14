"""Copy a verified staged runtime into an app-local, relocatable resource tree."""
import argparse,json,shutil,subprocess
from pathlib import Path

def sanitize_runtime(destination):
    manifest=destination/'runtime-manifest.json'
    data=json.loads(manifest.read_text())
    # AppKit owns the UI. Tk is unused by inference and carries private AppKit selectors.
    lib=destination/'python/lib'
    paths=[]
    for pattern in ['libtcl*.dylib','libtk*.dylib','tcl[0-9]*','tk[0-9]*','itcl[0-9]*','thread[0-9]*']:
        paths.extend(lib.glob(pattern))
    stdlib=lib/'python3.12'
    paths.extend((stdlib/'lib-dynload').glob('_tkinter*.so'))
    paths.extend([stdlib/'tkinter',stdlib/'turtledemo',stdlib/'turtle.py'])
    paths.extend((destination/'site-packages/PIL').glob('_imagingtk*.so'))
    paths.append(destination/'site-packages/PIL/ImageTk.py')
    for path in paths:
        if path.is_symlink() or path.is_file():path.unlink()
        elif path.is_dir():shutil.rmtree(path)
    data['excludedOptionalComponents']=['Tcl/Tk','tkinter','PIL.ImageTk']
    manifest.write_text(json.dumps(data,indent=2)+'\n')

p=argparse.ArgumentParser();p.add_argument('--venv');p.add_argument('--output',required=True);p.add_argument('--sanitize-only',action='store_true');a=p.parse_args()
if a.sanitize_only:
    sanitize_runtime(Path(a.output))
    print('Sanitized App Store runtime: removed unused Tcl/Tk GUI components')
    raise SystemExit(0)
if not a.venv:p.error('--venv is required when staging a new runtime')
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
sanitize_runtime(destination)
# Keep every symlink app-relative. Standalone CPython's internal aliases are valid.
for path in destination.rglob('*'):
    if path.is_symlink() and str(path.readlink()).startswith('/'):
        raise SystemExit('Runtime contains an absolute symlink: '+str(path.relative_to(destination)))
print(destination)
