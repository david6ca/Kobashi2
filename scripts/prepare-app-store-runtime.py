"""Copy a verified staged runtime into an app-local, relocatable resource tree."""
import argparse,hashlib,json,shutil,subprocess,tempfile,urllib.request,zipfile
from pathlib import Path,PurePosixPath

SCIPY_VERSION='1.18.1'
SCIPY_WHEEL='scipy-1.18.1-cp312-cp312-macosx_12_0_arm64.whl'
SCIPY_URL='https://files.pythonhosted.org/packages/05/4a/78c6285577c375e7cf27277ea8ee6961224327f1e1a0c44af5f17f23635c/'+SCIPY_WHEEL
SCIPY_SHA256='e708533e8b2ae2497d65346538a7dcc92814410b25b81432eac66de0f2af8265'
SDL_ARCHIVE='SDL2-2.32.8.tar.gz'
SDL_URL='https://github.com/libsdl-org/SDL/releases/download/release-2.32.8/'+SDL_ARCHIVE
SDL_SHA256='0ca83e9c9b31e18288c7ec811108e58bac1f1bb5ec6577ad386830eac51c787e'

def verified_download(name,url,digest):
    cache=Path('/private/tmp/modelbridge-public-api-wheels');cache.mkdir(mode=0o700,parents=True,exist_ok=True)
    path=cache/name
    if not path.exists() or hashlib.sha256(path.read_bytes()).hexdigest()!=digest:
        temporary=path.with_suffix('.download')
        with urllib.request.urlopen(url,timeout=60) as response,temporary.open('wb') as output:shutil.copyfileobj(response,output)
        if hashlib.sha256(temporary.read_bytes()).hexdigest()!=digest:raise SystemExit('App Store dependency checksum mismatch: '+name)
        temporary.replace(path)
    return path

def select_public_sdl(destination,data):
    target=destination/'site-packages/cv2/.dylibs/libSDL2-2.0.0.dylib'
    if not target.exists():return
    if data.get('appStoreSDL',{}).get('sourceSha256')==SDL_SHA256 and b'setAppleMenu:' not in target.read_bytes():return
    archive=verified_download(SDL_ARCHIVE,SDL_URL,SDL_SHA256)
    source=archive.parent/'sdl-source';source.mkdir(exist_ok=True)
    if not (source/'SDL2-2.32.8/CMakeLists.txt').is_file():
        subprocess.run(['/usr/bin/tar','-xzf',str(archive),'-C',str(source)],check=True)
    build=archive.parent/'sdl-build'
    options=['-DCMAKE_BUILD_TYPE=Release','-DCMAKE_OSX_ARCHITECTURES=arm64','-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0','-DSDL_SHARED=ON','-DSDL_STATIC=OFF','-DSDL_TEST=OFF','-DSDL_TESTS=OFF','-DSDL_COCOA=OFF','-DSDL_RENDER_METAL=OFF','-DSDL_X11=OFF','-DSDL_WAYLAND=OFF']
    subprocess.run(['cmake','-S',str(source/'SDL2-2.32.8'),'-B',str(build),'-G','Ninja']+options,check=True)
    subprocess.run(['cmake','--build',str(build),'--parallel','6'],check=True)
    library=build/'libSDL2-2.0.0.dylib'
    if b'setAppleMenu:' in library.read_bytes():raise SystemExit('SDL still contains the private Cocoa selector')
    shutil.copy2(library,target)
    data['appStoreSDL']={'version':'2.32.8','sourceSha256':SDL_SHA256,'cocoaVideo':False,'metalRenderer':False}

def select_public_scipy(destination,data):
    site=destination/'site-packages'
    version=next(p['Version'] for p in data['packages'] if p['Name'].lower()=='scipy')
    if version!=SCIPY_VERSION:raise SystemExit('Update the audited App Store SciPy wheel for version '+version)
    if data.get('appStoreSciPyWheel',{}).get('sha256')==SCIPY_SHA256 and (site/'scipy/.dylibs/libscipy_openblas.dylib').is_file():return
    # The same SciPy release has separate official OpenBLAS and Accelerate wheels.
    wheel=verified_download(SCIPY_WHEEL,SCIPY_URL,SCIPY_SHA256)
    with tempfile.TemporaryDirectory(prefix='scipy-overlay-',dir=destination.parent) as folder:
        overlay=Path(folder)
        with zipfile.ZipFile(wheel) as archive:
            for name in archive.namelist():
                path=PurePosixPath(name)
                if path.is_absolute() or '..' in path.parts or path.parts[0] not in ['scipy','scipy-'+SCIPY_VERSION+'.dist-info']:
                    raise SystemExit('Unexpected path in App Store SciPy wheel: '+name)
            archive.extractall(overlay)
        for name in ['scipy','scipy-'+SCIPY_VERSION+'.dist-info']:
            target=site/name
            if target.is_dir():shutil.rmtree(target)
            (overlay/name).replace(target)
    data['appStoreSciPyWheel']={'filename':SCIPY_WHEEL,'sha256':SCIPY_SHA256,'backend':'OpenBLAS'}

def sanitize_runtime(destination):
    manifest=destination/'runtime-manifest.json'
    data=json.loads(manifest.read_text())
    select_public_scipy(destination,data)
    select_public_sdl(destination,data)
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
    # S3 is an optional Arrow filesystem; model downloads use Hugging Face HTTPS.
    paths.extend((destination/'site-packages/pyarrow').glob('_s3fs.*'))
    paths.extend((destination/'site-packages/pyarrow').glob('libarrow_s3.*.dylib'))
    for path in paths:
        if path.is_symlink() or path.is_file():path.unlink()
        elif path.is_dir():shutil.rmtree(path)
    data['excludedOptionalComponents']=['Tcl/Tk','tkinter','PIL.ImageTk','PyArrow S3']
    manifest.write_text(json.dumps(data,indent=2)+'\n')

p=argparse.ArgumentParser();p.add_argument('--venv');p.add_argument('--output',required=True);p.add_argument('--sanitize-only',action='store_true');a=p.parse_args()
if a.sanitize_only:
    sanitize_runtime(Path(a.output))
    print('Sanitized App Store runtime: OpenBLAS SciPy; SDL without Cocoa; unused Tk and Arrow S3 removed')
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
