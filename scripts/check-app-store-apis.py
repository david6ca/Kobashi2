"""Reject the specific non-public/deprecated API references identified in App Review."""
import argparse,concurrent.futures,subprocess
from pathlib import Path

MAGIC={bytes.fromhex(value) for value in ['cffaedfe','cefaedfe','feedfacf','feedface','cafebabe','bebafeca','cafebabf','bfbafeca']}

def inspect(path,forbidden):
    result=subprocess.run(['nm','-m',str(path)],capture_output=True,text=True)
    if result.returncode:raise RuntimeError('Cannot inspect '+str(path)+': '+result.stderr)
    hits=set()
    for line in result.stdout.splitlines():hits.update(forbidden.intersection(line.split()))
    data=path.read_bytes()
    for symbol in forbidden:
        if b'\0'+symbol.encode()+b'\0' in data:hits.add(symbol)
    return path,sorted(hits)

def main():
    parser=argparse.ArgumentParser();parser.add_argument('root',type=Path);args=parser.parse_args()
    definitions=Path(__file__).resolve().parents[1]/'macos/app-store/nonpublic-api-symbols.txt'
    forbidden={line.strip() for line in definitions.read_text().splitlines() if line.strip() and not line.startswith('#')}
    binaries=[]
    for path in sorted(args.root.rglob('*')):
        if not path.is_file() or path.is_symlink():continue
        with path.open('rb') as stream:
            if stream.read(4) in MAGIC:binaries.append(path)
    if not binaries:raise SystemExit('No Mach-O binaries found under '+str(args.root))
    failures=[]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        for path,hits in pool.map(lambda path:inspect(path,forbidden),binaries):
            if hits:failures.append((path,hits))
    for path,hits in failures:print(str(path.relative_to(args.root))+': '+', '.join(hits))
    if failures:raise SystemExit('App Review API check failed; do not export/upload this candidate')
    print(f'PASS: {len(binaries)} Mach-O binaries have no references to {len(forbidden)} tracked App Review API names')

if __name__=='__main__':main()
