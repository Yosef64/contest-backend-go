"""Integration checks. Run only against a disposable development database.
python scripts/core_services_check.py http://localhost:18090
"""
import json,sys,urllib.request,urllib.error
from http.cookiejar import CookieJar
from datetime import datetime,timedelta,timezone
RUN=datetime.now().strftime("%H%M%S%f");base=sys.argv[1];cookie=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(CookieJar()))
def call(method,path,body=None,token=None,admin=False):
 req=urllib.request.Request(base+'/api/v1/'+path,data=None if body is None else json.dumps(body).encode(),method=method,headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else {})})
 try:
  with (cookie if admin else urllib.request.build_opener()).open(req,timeout=10) as r:
   raw=r.read();return r.status,json.loads(raw) if raw and 'json' in r.headers.get('Content-Type','') else raw.decode()
 except urllib.error.HTTPError as e:return e.code,json.loads(e.read())

def ok(name,status,want):
 assert status==want,(name,status,want);print('PASS',name)
def student(name):
 status,r=call('POST','auth/dev',{'id':name+RUN,'name':name+RUN});ok('dev login '+name,status,200);t=r['token']
 status,r=call('PUT','me',{'name':name,'phone_number':'0911000000','gender':'female','age':18,'school':'Test school','grade':'12','city':'Adama','region':'Oromia','department':'entrance','stream':'natural'},t);ok('profile '+name,status,200);return t,r['student']['id']
def admin(path,body=None,method='POST'):return call(method,'admin/'+path,body,admin=True)
def iso(t):return t.isoformat().replace('+00:00','Z')
status,r=admin('auth/login',{'email':'core-test@victory.local','password':'Core-test-owner-2026'});ok('owner login',status,200)
t,uid=student('Core student');other,oid=student('Other student')
status,r=admin('services',{'kind':'school','title':'Core school'});ok('create school',status,204)
status,svc=admin('services',method='GET');ok('service inventory',status,200);school=svc['schools'][0]
ok('school join',call('PUT','learning-hub/profile',{'department':'entrance','stream':'natural','school_code':school['join_code']},t)[0],204)
ok('academic profile',call('PUT','learning-hub/profile',{'department':'entrance','stream':'natural'},other)[0],204)
ok('bad join code rejected',call('PUT','learning-hub/profile',{'department':'entrance','stream':'natural','school_code':'invalid'},other)[0],422)
status,r=admin('courses',{'title':'Core course','description':'Test','grade':'12','subject':'Mathematics'});ok('create course',status,201);course=r['course']['id']
status,r=admin('courses/'+course+'/chapters',{'title':'Core chapter'});ok('create chapter',status,201);chapter=r['chapter']['id']
status,r=admin('chapters/'+chapter,method='GET');v=r['chapter']['version']
ok('publish chapter',admin('chapters/'+chapter,{'html':'<p>Core lesson</p>','status':'published','base_version':v},'PUT')[0],200)
ok('assign paid course',admin('services',{'kind':'policy','resource_type':'course','resource_id':course,'access':'academic'})[0],204)
ok('paid access denied before enrollment',call('GET','chapters/'+chapter,token=t)[0],403)
ok('request enrollment',call('POST','learning-hub/enrollments',{'module':'academic','reference':'Bank-123'},t)[0],204)
ok('duplicate pending enrollment blocked',call('POST','learning-hub/enrollments',{'module':'academic','reference':'Bank-123'},t)[0],422 if False else 409)
_,svc=admin('services',method='GET');request=next(x for x in svc['enrollments'] if x['student_id']==uid)
ok('approve enrollment',admin('services',{'kind':'enrollment','id':request['id'],'status':'approved','days':30})[0],204)
ok('paid access allowed',call('GET','chapters/'+chapter,token=t)[0],200)
ok('assign school restriction',admin('services',{'kind':'policy','resource_type':'course','resource_id':course,'access':'free','school_id':school['id']})[0],204)
ok('other school direct link denied',call('GET','chapters/'+chapter,token=other)[0],403)
ok('target save',call('PUT','learning-hub/targets',{'subject':'Mathematics','target':85},t)[0],204)
ok('invalid target rejected',call('PUT','learning-hub/targets',{'subject':'Mathematics','target':110},t)[0],422)
ok('publish targeted notice',admin('services',{'kind':'announcement','title':'School notice','body':'Private school message','school_id':school['id'],'active':True})[0],204)
_,hub=call('GET','learning-hub',token=other);assert not any(x['title']=='School notice' for x in hub['announcements']);print('PASS notice school isolation')
ok('create habit program',admin('services',{'kind':'program','title':'Daily habits '+RUN,'tasks':['Read','Practice'],'active':True})[0],204)
_,svc=admin('services',method='GET');program=next(p['id'] for p in svc['programs'] if p['title']=='Daily habits '+RUN)
ok('training requires separate enrollment',call('PUT','learning-hub/habits/'+program,{'checked':[0]},t)[0],403)
ok('training request',call('POST','learning-hub/enrollments',{'module':'training','reference':'Training-123'},t)[0],204)
_,svc=admin('services',method='GET');request=next(x for x in svc['enrollments'] if x['module']=='training' and x['student_id']==uid)
ok('approve training',admin('services',{'kind':'enrollment','id':request['id'],'status':'approved','days':30})[0],204)
ok('duplicate checklist entries rejected',call('PUT','learning-hub/habits/'+program,{'checked':[0,0]},t)[0],400)
ok('daily checklist',call('PUT','learning-hub/habits/'+program,{'checked':[0,1]},t)[0],204)
ok('idempotent checklist update',call('PUT','learning-hub/habits/'+program,{'checked':[0]},t)[0],204)
_,hub=call('GET','learning-hub',token=t);assert len(next(p['logs'] for p in hub['programs'] if p['id']==program))==1;print('PASS single daily record')
ok('agent application',call('POST','learning-hub/agent',{},t)[0],204)
ok('self-referral rejected',call('POST','learning-hub/referrals',{'owner_id':uid,'chapter_id':chapter},t)[0],400)
status,r=admin('questions',{'text':'2+2','options':['4','5'],'answer_index':0,'subject':'Mathematics','chapter':'Arithmetic','grade':'12','explanation':'Four'});ok('create question',status,201);qid=r['question']['id']
now=datetime.now(timezone.utc)
status,r=admin('contests',{'title':'Core ended exam','subject':'Mathematics','grade':'12','start_time':iso(now-timedelta(hours=2)),'end_time':iso(now-timedelta(hours=1)),'question_ids':[qid]});ok('create ended exam',status,201);cid=r['contest']['id']
ok('enable makeup',admin('services/makeup',{'contest_id':cid,'minutes':30,'available_until':iso(now+timedelta(hours=1))},'PUT')[0],204)
status,r=call('POST','contests/'+cid+'/makeup',{},t);ok('start makeup',status,200);deadline=r['ends_at'];assert 'answer_index' not in r['questions'][0]
status,r=call('POST','contests/'+cid+'/makeup',{},t);assert r['ends_at']==deadline;print('PASS reopening preserves makeup timer')
ok('close new makeup attempts',admin('services/makeup/'+cid,method='DELETE')[0],204)
ok('closed makeup blocks new students',call('POST','contests/'+cid+'/makeup',{},other)[0],409)
status,r=call('POST','contests/'+cid+'/makeup',{},t);ok('closed makeup allows existing resume',status,200);assert r['ends_at']==deadline
_,hub=call('GET','learning-hub',token=t);assert any(m['contest_id']==cid for m in hub['makeup']);print('PASS closed schedule keeps active attempt discoverable')
ok('update makeup duration',admin('services/makeup',{'contest_id':cid,'minutes':60,'available_until':iso(now+timedelta(hours=2))},'PUT')[0],204)
_,r=call('POST','contests/'+cid+'/makeup',{},t);assert r['ends_at']==deadline;print('PASS settings changes preserve active deadlines')
ok('submit makeup',call('POST','contests/'+cid+'/makeup/submit',{'answers':{qid:0}},t)[0],201)
ok('repeat makeup submission rejected',call('POST','contests/'+cid+'/makeup/submit',{'answers':{qid:0}},t)[0],409)
ok('used question bulk deletion blocked',admin('questions/batch',{'ids':[qid],'action':'delete'})[0],409)
status,r=admin('services/reports?group=gender',method='GET');ok('gender report',status,200);assert r['rows'][0]['accuracy']==100
status,r=admin('services/reports?group=gender&grade=9',method='GET');ok('report grade filter',status,200);assert r['rows']==[]
status,r=admin('services/reports?group=gender&subject=Physics',method='GET');ok('report subject filter',status,200);assert r['rows']==[]
status,r=admin('services/reports?group=gender&from=2020-01-01&to=2020-01-01',method='GET');ok('report date filter',status,200);assert r['rows']==[]
status,r=admin('services/reports?group=gender&grade=9&format=csv',method='GET');ok('CSV obeys same filters',status,200);assert len(r.strip().splitlines())==1
ok('invalid report grade rejected',admin('services/reports?grade=99',method='GET')[0],400)
ok('invalid report subject rejected',admin('services/reports?subject=Unknown',method='GET')[0],400)
ok('invalid report dates rejected',admin('services/reports?from=2026-10-04&to=2026-10-03',method='GET')[0],400)
ok('invalid report dimension',admin('services/reports?group=password_hash',method='GET')[0],400)
ok('CSV report',admin('services/reports?group=school&format=csv',method='GET')[0],200)
# Positive referral unlock, bulk import, offline lease and password exam checks.
third,thirdid=student('Third student')
ok('enable referrals',admin('services',{'kind':'policy','resource_type':'chapter','resource_id':chapter,'access':'free','referral_enabled':True})[0],204)
ok('referral locked initially',call('GET','chapters/'+chapter,token=t)[0],403)
for visitor in [other,other,third]:
 ok('authenticated referral visit',call('POST','learning-hub/referrals',{'owner_id':uid,'chapter_id':chapter},visitor)[0],204)
status,r=call('GET','chapters/'+chapter,token=t);ok('two distinct visits unlock',status,200)
assert datetime.fromisoformat(r['offline_until'].replace('Z','+00:00')) <= datetime.now(timezone.utc)+timedelta(hours=24,seconds=5)
print('PASS offline lease capped')
ok('bulk lesson import',admin('services/chapters/import',{'chapters':[{'course_id':course,'title':'Imported lesson','html':'<p>Lesson</p><script>bad()</script>','minutes':5,'status':'published'}]})[0],201)
_,before=admin('courses/'+course,method='GET')
ok('invalid bulk target rejected',admin('services/chapters/import',{'chapters':[{'course_id':course,'title':'Must rollback','minutes':5,'status':'draft','html':'<p>Safe</p>'},{'course_id':'missing-course','title':'Bad target','minutes':5,'status':'draft','html':''}]})[0],422)
_,after=admin('courses/'+course,method='GET');assert len(before['chapters'])==len(after['chapters']);print('PASS invalid import leaves existing content unchanged')
_,svc=admin('services',method='GET');assert any(e['action']=='makeup.close' and e['resource']==cid for e in svc['audit']);print('PASS makeup changes are audited')
status,r=admin('contests',{'title':'Password exam '+RUN,'subject':'Mathematics','grade':'12','start_time':iso(now-timedelta(minutes=5)),'end_time':iso(now+timedelta(hours=1)),'question_ids':[qid]});ok('create live exam',status,201);live=r['contest']['id']
ok('configure exam mode',admin('services/exam-mode',{'contest_id':live,'entry_password':'Enter-test-2026','exit_password':'Exit-test-2026','enabled':True},'PUT')[0],204)
ok('register live exam',call('POST','contests/'+live+'/register',{},t)[0],204)
ok('questions require entry password',call('GET','contests/'+live+'/questions',token=t)[0],403)
ok('incorrect entry rejected',call('POST','contests/'+live+'/exam-mode/enter',{'password':'incorrect'},t)[0],403)
ok('correct exam entry',call('POST','contests/'+live+'/exam-mode/enter',{'password':'Enter-test-2026'},t)[0],204)
ok('entered student receives questions',call('GET','contests/'+live+'/questions',token=t)[0],200)
ok('live scores hidden',call('GET','contests/'+live+'/standings',token=t)[0],409)
ok('wrong exit rejected',call('POST','contests/'+live+'/exam-mode/exit',{'password':'incorrect'},t)[0],403)
ok('disable exam mode',admin('services/exam-mode',{'contest_id':live,'enabled':False},'PUT')[0],204)
ok('exit remains possible after disable',call('POST','contests/'+live+'/exam-mode/exit',{'password':'Exit-test-2026'},t)[0],204)
ok('department leaderboard',call('GET','leaderboard',token=t)[0],200)
status,r=admin('auth/register',{'name':'Read only '+RUN,'email':'analyst-core-'+RUN+'@victory.local','password':'Read-only-test-2026'});ok('register analyst',status,201)
_,svc=admin('services',method='GET');role=next(a for a in svc['roles'] if a['name']=='Read only '+RUN)
ok('approve analyst',admin('admins/'+role['id'],{'is_approved':True},'PATCH')[0],204)
ok('assign analyst role',admin('services',{'kind':'role','id':role['id'],'role':'analyst'})[0],204)
ok('analyst login',admin('auth/login',{'email':'analyst-core-'+RUN+'@victory.local','password':'Read-only-test-2026'})[0],200)
ok('analyst write blocked',admin('services',{'kind':'school','title':'Unauthorized'})[0],403)
ok('owner relogin',admin('auth/login',{'email':'core-test@victory.local','password':'Core-test-owner-2026'})[0],200)
ok('register school manager',admin('auth/register',{'name':'School manager '+RUN,'email':'manager-core-'+RUN+'@victory.local','password':'Manager-test-2026'})[0],201)
_,svc=admin('services',method='GET');manager=next(a for a in svc['roles'] if a['name']=='School manager '+RUN)
ok('approve school manager',admin('admins/'+manager['id'],{'is_approved':True},'PATCH')[0],204)
ok('assign manager school',admin('services',{'kind':'role','id':manager['id'],'role':'manager','school_id':school['id']})[0],204)
ok('manager login',admin('auth/login',{'email':'manager-core-'+RUN+'@victory.local','password':'Manager-test-2026'})[0],200)
ok('manager global inventory denied',admin('services',method='GET')[0],403)
status,report=admin('services/reports?group=gender&school_id=00000000-0000-0000-0000-000000000000',method='GET');ok('manager report ignores attempted school override',status,200)
assert sum(r['students'] for r in report['rows']) >= 1
ok('manager global students denied',admin('students',method='GET')[0],403)
print('ALL CORE SERVICE CHECKS PASSED')

_,hub=call('GET','learning-hub',token=t)
performance=hub['performance']
row=next(x for x in performance['chapters'] if x['chapter']=='Arithmetic')
assert row['correct']==1 and row['incorrect']==0 and row['accuracy']==100
assert any(x['title']=='Core ended exam' for x in performance['history'])
habit=next(x for x in performance['habits'] if x['program_id']==program)
assert habit['active_days']==1 and habit['complete_days']==0 and habit['streak']==0
assert habit['today_checked']==1 and habit['week_checked']==1 and habit['rank']==1
ok('complete daily tasks',call('PUT','learning-hub/habits/'+program,{'checked':[0,1]},t)[0],204)
_,hub=call('GET','learning-hub',token=t)
habit=next(x for x in hub['performance']['habits'] if x['program_id']==program)
assert habit['complete_days']==1 and habit['streak']==1
_,peerhub=call('GET','learning-hub',token=other)
assert peerhub['performance']['chapters']==[] and peerhub['performance']['history']==[]
status,r=admin('services/reports?group=chapter&subject=Mathematics',method='GET')
ok('chapter breakdown report',status,200)
assert any(x['group']=='Mathematics / Arithmetic' and x['accuracy']==100 for x in r['rows'])
status,r=admin('services/reports?group=chapter&format=csv',method='GET')
ok('chapter CSV',status,200);assert 'Mathematics / Arithmetic' in r
print('PASS student analytics, habit metrics, streak and peer privacy')
