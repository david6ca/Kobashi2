"""Build/export an independent Mac App Store candidate through Xcode CLI.
Never installs the candidate, launches the ordinary app, or uploads a build.
"""
import argparse,json,os,plistlib,shutil,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--runtime-venv',required=True);p.add_argument('--output',required=True);p.add_argument('--team',required=True);p.add_argument('--build',default='1');p.add_argument('--resume',action='store_true');p.add_argument('--unsigned',action='store_true');a=p.parse_args()
root=Path(__file__).resolve().parents[1];output=Path(a.output).resolve()
if output==root or root in output.parents:raise SystemExit('App Store build output must be outside the checkout')
output.mkdir(parents=True,exist_ok=True);source=output/'source'
if source.exists() and not a.resume:raise SystemExit('Use a new candidate output directory, or --resume the existing candidate')
source.mkdir(exist_ok=True)
for name in ['cmd','internal','macos','scripts','assets']:
    shutil.copytree(root/name,source/name,dirs_exist_ok=True,ignore=shutil.ignore_patterns('__pycache__','xcuserdata','*.xcuserstate'))
for name in ['go.mod','go.sum','package.json']:shutil.copyfile(root/name,source/name)
store_metadata=source/'metadata/app-store/metadata.json'
store_metadata.parent.mkdir(parents=True,exist_ok=True)
shutil.copyfile(root/'metadata/app-store/metadata.json',store_metadata)
store=source/'macos/app-store'
if not (store/'Runtime/runtime-manifest.json').exists():
    subprocess.run(['python3',str(root/'scripts/prepare-app-store-runtime.py'),'--venv',a.runtime_venv,'--output',str(store/'Runtime')],check=True)
else:
    subprocess.run(['python3',str(root/'scripts/prepare-app-store-runtime.py'),'--output',str(store/'Runtime'),'--sanitize-only'],check=True)
# Generate the same product icon without altering tracked artwork or other builds.
iconset=output/'AppIcon.iconset'
subprocess.run(['xcrun','--toolchain','XcodeDefault','swift','-module-cache-path',str(output/'swift-cache'),str(source/'macos/icon.swift'),str(source/'assets/kobashi-icon.png'),str(iconset)],check=True)
subprocess.run(['/usr/bin/iconutil','-c','icns',str(iconset),'-o',str(store/'AppIcon.icns')],check=True)
version=json.loads(store_metadata.read_text())['version']
project=store/'ModelBridge.xcodeproj';archive=output/'ModelBridge.xcarchive'
command=['xcodebuild','-project',str(project),'-scheme','ModelBridge','-configuration','Release','-destination','generic/platform=macOS','-archivePath',str(archive),'-derivedDataPath',str(output/'DerivedData'),'DEVELOPMENT_TEAM='+a.team,'MARKETING_VERSION='+version,'CURRENT_PROJECT_VERSION='+a.build,'-allowProvisioningUpdates','archive']
if a.unsigned:command+=['CODE_SIGNING_ALLOWED=NO']
with (output/'archive.log').open('w') as log:
    subprocess.run(command,stdout=log,stderr=subprocess.STDOUT,check=True)
app=archive/'Products/Applications/ModelBridge.app'
subprocess.run(['python3',str(root/'scripts/check-app-store-apis.py'),str(app)],check=True)
if a.unsigned:
    subprocess.run(['/usr/bin/codesign','--force','--sign','-','--entitlements',str(store/'ModelBridge.entitlements'),str(app)],check=True)
else:
    export=output/'export'
    options={'method':'app-store-connect','destination':'export','signingStyle':'automatic','teamID':a.team,'manageAppVersionAndBuildNumber':False,'uploadSymbols':True}
    with (output/'ExportOptions.plist').open('wb') as f:plistlib.dump(options,f)
    with (output/'export.log').open('w') as log:
        subprocess.run(['xcodebuild','-exportArchive','-archivePath',str(archive),'-exportPath',str(export),'-exportOptionsPlist',str(output/'ExportOptions.plist'),'-allowProvisioningUpdates'],stdout=log,stderr=subprocess.STDOUT,check=True)
subprocess.run(['/usr/bin/codesign','--verify','--deep','--strict',str(app)],check=True)
subprocess.run([str(app/'Contents/MacOS/ModelBridge'),'--store-self-test'],check=True)
commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
dirty=bool(subprocess.check_output(['git','status','--porcelain'],cwd=root,text=True).strip())
receipt={'productName':'ModelBridge','bundleId':'com.pomegram.modelbridge','version':version,'build':a.build,'sourceCommit':commit,'sourceDirty':dirty,'appSandbox':True,'runtimeBundled':True,'uploaded':False,'mainInstalledAppReplaced':False,'screenshotsUpdated':False,'archive':str(archive)}
(output/'candidate.json').write_text(json.dumps(receipt,indent=2)+'\n')
print(json.dumps(receipt,indent=2))
