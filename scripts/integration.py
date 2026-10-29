#!/usr/bin/env python3
"""Run the actual CPA C ABI host with synthetic credentials and a local upstream."""
import argparse, contextlib, hashlib, http.server, json, os, pathlib, platform, shutil, signal, socket, subprocess, threading, time, urllib.error, urllib.parse, urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[1]
KEY='synthetic-quality-management'
BASE='http://127.0.0.1:28417'
PREFIX='/v0/management/plugins/model-quality/'
OPENER=urllib.request.build_opener(urllib.request.ProxyHandler({}))
CAPTURES=[];CASES=[];WAIT=threading.Event()

class Mock(http.server.BaseHTTPRequestHandler):
 protocol_version='HTTP/1.1'
 def log_message(self,*_):pass
 def do_GET(self):
  data=json.dumps({'data':[{'id':'quality-test'}]}).encode();self.send_response(200);self.send_header('Content-Length',str(len(data)));self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(data)
 def do_POST(self):
  data=json.loads(self.rfile.read(int(self.headers.get('Content-Length',0))))
  def content_text(content):
   if isinstance(content,str):return content
   return '\n'.join(p.get('text','') for p in content if isinstance(p,dict)) if isinstance(content,list) else ''
  messages=data.get('messages',data.get('input',[]))
  prompt=content_text(messages) if isinstance(messages,str) else '\n'.join(content_text(m.get('content','')) for m in messages)
  CAPTURES.append({'key':self.headers.get('Authorization'),'prompt':prompt,'path':self.path})
  if self.path.rstrip('/').endswith('/responses'):
   self.send_response(200);self.send_header('Content-Type','text/event-stream');self.send_header('Connection','close');self.end_headers()
   for event in [
    {'type':'response.created','response':{'id':'synthetic-native','model':'quality-native','status':'in_progress'}},
    {'type':'response.output_text.delta','delta':'NATIVE_SYNTHETIC_OK'},
    {'type':'response.completed','response':{'id':'synthetic-native','model':'quality-native','status':'completed','output':[{'type':'message','role':'assistant','content':[{'type':'output_text','text':'NATIVE_SYNTHETIC_OK'}]}],'usage':{'input_tokens':12,'output_tokens':6,'total_tokens':18}}}]:
    self.wfile.write(('event: '+event['type']+'\ndata: '+json.dumps(event)+'\n\n').encode());self.wfile.flush()
   self.close_connection=True;return
  if 'MOCK_PIN_FAILURE' in prompt and self.headers.get('Authorization')=='Bearer synthetic-account-a':
   body=b'{"error":{"message":"synthetic pinned credential failure","type":"invalid_api_key"}}'
   self.send_response(401);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body);return
  selected=next((c for c in CASES if c['prompt'] in prompt),None)
  if 'MOCK_WAIT' in prompt:WAIT.clear();reply='stream started';finish='stop'
  elif 'MOCK_TRUNCATED' in prompt:reply='{"partial":true}';finish='length'
  elif selected and selected['kind']=='json':reply=json.dumps(selected['expected'],ensure_ascii=False);finish='stop'
  elif selected and selected['kind']=='choice':reply='Reasoning.\nFINAL: '+selected['expected']['answer'];finish='stop'
  elif selected and selected['kind']=='visual':reply='<!doctype html><html><body><svg viewBox="0 0 400 220"><circle cx="100" cy="150" r="40" stroke="#4878f5" fill="none"/><circle cx="300" cy="150" r="40" stroke="#4878f5" fill="none"/><path d="M100 150L200 60 300 150Z" stroke="#4878f5" fill="none"/></svg><script>window.syntheticAnimation=true</script></body></html>';finish='stop'
  else:reply='{"synthetic":true}';finish='stop'
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.send_header('Cache-Control','no-cache');self.send_header('Connection','close');self.end_headers()
  def emit(content=None,reason=None,usage=None):
   item={'id':'synthetic-chat','object':'chat.completion.chunk','created':1700000000,'model':'quality-test-reported','choices':[{'index':0,'delta':({'role':'assistant','content':content} if content is not None else {}),'finish_reason':reason}]}
   if usage:item['usage']=usage
   self.wfile.write(('data: '+json.dumps(item,ensure_ascii=False)+'\n\n').encode());self.wfile.flush()
  try:
   for i in range(0,len(reply),23):emit(reply[i:i+23])
   if 'MOCK_WAIT' in prompt:WAIT.wait(30)
   emit(reason=finish,usage={'prompt_tokens':90,'completion_tokens':25,'total_tokens':115});self.wfile.write(b'data: [DONE]\n\n');self.wfile.flush()
  except (BrokenPipeError,ConnectionResetError):pass
  self.close_connection=True

def request(path,body=None,method='GET',auth=True):
 headers={'Content-Type':'application/json'}
 if auth:headers['Authorization']='Bearer '+KEY
 req=urllib.request.Request(BASE+path,data=json.dumps(body).encode() if body is not None else None,method=method,headers=headers)
 with OPENER.open(req,timeout=15) as r:return json.load(r)
def api(path,body=None,method='GET'):return request(PREFIX+path,body,method)
def wait_batch(batch):
 for _ in range(100):
  b=api('batch?id='+batch['id'])
  if b['status'] not in ['running','cancelling']:return b
  time.sleep(.05)
 raise AssertionError('synthetic batch did not terminate')

def main():
 global CASES
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--keep',action='store_true');p.add_argument('--image',default='eceasy/cli-proxy-api:v8.0.47');args=p.parse_args()
 stage=ROOT/'build/integration';stage.mkdir(parents=True,exist_ok=True);os.chmod(stage,0o700)
 arch={'x86_64':'amd64','aarch64':'arm64'}.get(platform.machine());assert arch,'Unsupported integration architecture'
 plugin=ROOT/'build/linux'/arch/'model-quality.so';assert plugin.is_file(),'Build the native library first'
 for port in [28417,28418]:
  with socket.socket() as s:s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('127.0.0.1',port))
 plugins=stage/'plugins/linux'/arch;plugins.mkdir(parents=True,exist_ok=True);shutil.copyfile(plugin,plugins/'model-quality.so')
 (stage/'data').mkdir(exist_ok=True);(stage/'auths').mkdir(exist_ok=True)
 config={'host':'127.0.0.1','port':28417,'auth-dir':'/quality/auths','api-keys':['synthetic-client-key'],'request-retry':0,'remote-management':{'allow-remote':False,'secret-key':KEY,'disable-control-panel':True},'openai-compatibility':[{'name':'quality-mock','base-url':'http://127.0.0.1:28418/v1','api-key-entries':[{'api-key':'synthetic-account-a'},{'api-key':'synthetic-account-b'}],'models':[{'name':'quality-test','alias':'quality-test'}]}],'plugins':{'enabled':True,'dir':'/quality/plugins','configs':{'model-quality':{'enabled':True,'data_dir':'/quality-data','retention_days':7}}}}
 config['codex-api-key']=[{'api-key':'synthetic-native-key','base-url':'http://127.0.0.1:28418','models':[{'name':'quality-native','alias':'quality-native'}]}]
 (stage/'config.yaml').write_text(json.dumps(config));os.chmod(stage/'config.yaml',0o600)
 name='cpa-model-quality-integration-'+str(os.getpid())
 server=http.server.ThreadingHTTPServer(('127.0.0.1',28418),Mock);threading.Thread(target=server.serve_forever,daemon=True).start()
 checks=[]
 def check(label):checks.append(label);print('PASS '+label,flush=True)
 try:
  subprocess.run(['docker','run','--detach','--name',name,'--user',f'{os.getuid()}:{os.getgid()}','--network','host','--mount',f'type=bind,src={stage},dst=/quality,readonly','--mount',f'type=bind,src={stage / "data"},dst=/quality-data',args.image,'./CLIProxyAPI','--config','/quality/config.yaml','--local-model'],check=True,capture_output=True)
  boot=None
  for _ in range(150):
   try:
    boot=api('bootstrap')
    if len(boot.get('accounts',[]))>=3:break
   except (urllib.error.URLError,TimeoutError):pass
   time.sleep(.1)
  assert boot and len(boot['accounts'])==3,boot
  CASES=boot['cases'];assert len(CASES)>=12
  check('C ABI plugin loads and discovers compatible and native runtime credentials')
  try:request(PREFIX+'bootstrap',auth=False);raise AssertionError('anonymous data endpoint allowed')
  except urllib.error.HTTPError as e:assert e.code in [401,403]
  check('account and history data require management authentication')
  with OPENER.open(BASE+'/v0/resource/plugins/model-quality/page') as r:assert b'MODEL QUALITY LAB' in r.read()
  check('static plugin page is navigable without exposing account data')
  targets=[{'auth_id':a['id'],'model':'quality-test'} for a in boot['accounts'] if 'quality-test' in a['models']]
  spec={'targets':targets,'case_ids':['probability','code','pelican','simplebench-4'],'effort':'auto','concurrency':2,'repeats':1}
  first=wait_batch(api('batches',spec,'POST'))
  assert len(first['items'])==8
  assert all(it['status']=='completed' for it in first['items']),[(it['status'],it.get('error')) for it in first['items']]
  assert all(it['grade']['score']==100 for it in first['items'] if it['case_id']!='pelican')
  check('actual host executes and translates streamed responses with objective grading')
  assert {c['key'] for c in CAPTURES}=={'Bearer synthetic-account-a','Bearer synthetic-account-b'}
  assert len(CAPTURES)==8
  assert all(not any(c['reference'] in r['prompt'] for c in CASES) for r in CAPTURES)
  check('both exact credentials execute; reference answers are not appended')
  visual=next(it for it in first['items'] if it['case_id']=='pelican')
  artifact=api('artifact?id='+first['id']+'&item='+visual['id']);assert artifact['html'].startswith('<!doctype html>') and not visual.get('grade')
  check('HTML artifact retained separately from automatic scores')
  retry=wait_batch(api('retry',{'id':first['id'],'items':[first['items'][0]['id']]},'POST'))
  assert retry['id']!=first['id'] and retry['retry_of']==first['id'] and api('batch?id='+first['id'])['items'][0]['status']=='completed'
  check('single-item retry creates independent immutable history')
  api('baseline',{'id':first['id']},'POST')
  assert api('batches')['baseline']==first['id']
  exported=api('export?id='+first['id']);serialized=json.dumps(exported)
  assert KEY not in serialized and 'synthetic-account-a' not in serialized and 'synthetic-account-b' not in serialized
  check('baseline and full JSON export contain no credentials')
  custom=dict(spec,targets=targets[:1],case_ids=['custom'],custom_prompt='MOCK_TRUNCATED',repeats=1)
  truncated=wait_batch(api('batches',custom,'POST'));assert truncated['items'][0]['status']=='failed' and not truncated['items'][0].get('grade'),truncated
  check('translated length truncation remains ungraded')
  custom.update(custom_prompt='MOCK_WAIT',repeats=3,concurrency=1)
  waiting=api('batches',custom,'POST')
  for _ in range(100):
   current=api('batch?id='+waiting['id'])
   if current['items'][0].get('preview'):break
   time.sleep(.05)
  api('cancel',{'id':waiting['id']},'POST');WAIT.set();cancelled=wait_batch(waiting)
  assert cancelled['status']=='cancelled' and all(it['status']=='cancelled' and not it.get('grade') for it in cancelled['items'])
  check('cancellation stops queued work and does not score partial replies')
  native=next(a for a in boot['accounts'] if 'quality-native' in a['models'])
  native_batch=wait_batch(api('batches',dict(spec,targets=[{'auth_id':native['id'],'model':'quality-native'}],case_ids=['custom'],custom_prompt='NATIVE_SYNTHETIC'),'POST'))
  native_export=api('export?id='+native_batch['id'])
  assert native_export['items'][0]['status']=='completed' and native_export['items'][0]['reply']=='NATIVE_SYNTHETIC_OK',native_export
  check('native Codex executor callback records preserve text and explicit completion')
  before=len(CAPTURES)
  failure=wait_batch(api('batches',dict(spec,case_ids=['custom'],custom_prompt='MOCK_PIN_FAILURE'),'POST'))
  assert sorted(it['status'] for it in failure['items'])==['completed','failed'],failure
  assert len(CAPTURES)-before==2 and {c['key'] for c in CAPTURES[before:]}=={'Bearer synthetic-account-a','Bearer synthetic-account-b'}
  check('a pinned credential failure never falls back to the other credential')
  subprocess.run(['docker','restart',name],check=True,capture_output=True)
  for _ in range(150):
   try:
    restarted=api('batches')
    if restarted['baseline']==first['id']:break
   except (urllib.error.URLError,TimeoutError):pass
   time.sleep(.1)
  assert restarted['baseline']==first['id']
  assert api('export?id='+first['id'])['items']==exported['items']
  assert api('batch?id='+cancelled['id'])['status']=='cancelled'
  check('core restart restores raw history, cancelled statuses and the selected baseline')
  (stage/'environment.json').write_text(json.dumps({'base':BASE,'key':KEY,'first_batch':first['id'],'container':name}))
  report={'image':args.image,'arch':arch,'library_sha256':hashlib.sha256(plugin.read_bytes()).hexdigest(),'checks':checks,'real_model_requests':0,'synthetic_upstream_requests':len(CAPTURES),'browser_environment_retained':args.keep}
  (stage/'verification.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report),flush=True)
  if args.keep:
   print('Synthetic environment retained until interrupted',flush=True)
   threading.Event().wait()
 finally:
  WAIT.set();subprocess.run(['docker','logs',name],stdout=(stage/'core.log').open('w'),stderr=subprocess.STDOUT)
  subprocess.run(['docker','rm','-f',name],capture_output=True);server.shutdown()

if __name__=='__main__':
 try:main()
 except KeyboardInterrupt:pass
