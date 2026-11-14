"""Stage the pinned native llama.cpp engine; no Python runtime is shipped."""
import argparse, shutil, subprocess
from pathlib import Path

parser=argparse.ArgumentParser()
parser.add_argument('--runtime-dir',type=Path)
parser.add_argument('--output',required=True,type=Path)
args=parser.parse_args()
root=Path(__file__).resolve().parents[1]
output=args.output.resolve()
if output.exists():
    shutil.rmtree(output)
source=args.runtime_dir
if source is None:
    source=Path(subprocess.check_output(['go','run','./cmd/app-store-runtime','--home',str(output.parent/'runtime-build-home')],cwd=root,text=True).strip())
if not (source/'llama-server').is_file() or not (source/'runtime-manifest.json').is_file():
    raise SystemExit('A prepared native llama.cpp runtime is required')
shutil.copytree(source,output,symlinks=True)
print(output)
