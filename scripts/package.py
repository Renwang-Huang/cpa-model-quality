#!/usr/bin/env python3
"""Package a native glibc C ABI library in the CPA store's zip layout."""
import argparse, hashlib, json, pathlib, re, zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--arch',choices=['amd64','arm64'],required=True);args=p.parse_args()
version=re.search(r'const pluginVersion\s*=\s*"([^"]+)"',(ROOT/'plugin.go').read_text())[1]
lib=ROOT/'build/linux'/args.arch/'model-quality.so'
raw=lib.read_bytes();assert raw[:4]==b'\x7fELF' and int.from_bytes(raw[18:20],'little')=={'amd64':62,'arm64':183}[args.arch]
out=ROOT/'dist';out.mkdir(exist_ok=True)
manifest={'id':'model-quality','version':version,'os':'linux','arch':args.arch,'libc':'glibc','abi':1,'schema':6,'verified_host_version':'v8.0.47','go_version':'go1.26.1','sha256':hashlib.sha256(raw).hexdigest()}
(out/f'build-manifest-{args.arch}.json').write_text(json.dumps(manifest,indent=2)+'\n')
target=out/f'model-quality_{version}_linux_{args.arch}.zip'
files=[(lib,'model-quality.so')]+[(ROOT/n,n) for n in ['README.md','LICENSE','THIRD_PARTY_NOTICES.md']]
files += [(f,str(f.relative_to(ROOT))) for f in sorted((ROOT/'docs').glob('*.md'))]
files += [(f,str(f.relative_to(ROOT))) for f in sorted((ROOT/'benchmarks').glob('*LICENSE'))]
files += [(f,str(f.relative_to(ROOT))) for f in sorted((ROOT/'licenses').iterdir()) if f.is_file()]
with zipfile.ZipFile(target,'w',compression=zipfile.ZIP_DEFLATED,compresslevel=9) as z:
 for f,n in files:
  info=zipfile.ZipInfo(n,(2026,10,29,0,0,0));info.compress_type=zipfile.ZIP_DEFLATED;info.external_attr=(0o100755 if n.endswith('.so') else 0o100644)<<16;z.writestr(info,f.read_bytes())
checks=[hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name for f in sorted(out.glob(f'model-quality_{version}_linux_*.zip'))]
(out/'checksums.txt').write_text('\n'.join(checks)+'\n');print(target)
