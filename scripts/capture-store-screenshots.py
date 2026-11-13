"""Capture only the mock preview's window IDs from the already authorized host."""
import argparse,json,os,subprocess,time,tempfile,shutil
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',required=True);p.add_argument('--locale');a=p.parse_args()
root=Path(__file__).resolve().parents[1];output=Path(a.output).resolve();output.mkdir(parents=True,exist_ok=True)
request=output/'.capture-request.json';request.unlink(missing_ok=True)
raw=Path(tempfile.mkdtemp(prefix='modelbridge-native-',dir=os.environ.get('TMPDIR','/private/var/tmp')))
env=dict(os.environ,KOBASHI_SCREENSHOT_OUTPUT=str(output),KOBASHI_SCREENSHOT_RAWDIR=str(raw))
if a.locale:env['KOBASHI_SCREENSHOT_LOCALE']=a.locale
process=subprocess.Popen(['go','run','./cmd/build','-store-screenshots'],cwd=root,env=env)
try:
    while process.poll() is None:
        if request.exists():
            value=json.loads(request.read_text());request.unlink()
            destination=Path(value['path'])
            if destination.parent!=raw:raise RuntimeError('Preview requested an unexpected destination')
            print('Own preview:',value,flush=True)
            subprocess.run(['/usr/sbin/screencapture','-x','-o','-l',str(value['windowID']),str(destination)],check=True)
        time.sleep(0.1)
    if process.returncode:raise SystemExit(process.returncode)
finally:
    request.unlink(missing_ok=True)
    shutil.rmtree(raw,ignore_errors=True)
    if process.poll() is None:
        process.wait(timeout=40)
