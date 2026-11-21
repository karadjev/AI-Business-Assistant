const {test}=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events');
const http=require('node:http'),https=require('node:https'),{run}=require('./smoke.cjs');
test('coordinator node-e execution runs the smoke instead of exiting silently',()=>{
 const {spawnSync}=require('node:child_process'),fs=require('node:fs');
 const result=spawnSync(process.execPath,['-e',fs.readFileSync(require.resolve('./smoke.cjs'),'utf8')],{input:JSON.stringify({origin:'http://invalid.test',ca:'public'}),encoding:'utf8'});
 assert.equal(result.status,1);assert.equal(result.stdout,'');assert.match(result.stderr,/private smoke failed/);
});
test('private smoke pins network targets and enforces TLS verification',async()=>{
 const old=[http.request,https.request],calls=[],statuses=[200,200,401,403];
 const fake=(opts,callback)=>{calls.push(opts);const req=new EventEmitter();req.end=()=>callback({statusCode:statuses.shift(),resume(){}});return req};
 try{http.request=fake;https.request=fake;
  assert.deepEqual(await run({origin:'https://staging.invalid:8443',ca:'synthetic public CA'}),{api_ready:200,tls_login:200,unauthenticated:401,csrf:403});
  assert.deepEqual(calls.map(c=>c.hostname),['api','proxy','proxy','proxy']);
  for(const c of calls.slice(1)){assert.equal(c.rejectUnauthorized,true);assert.equal(c.servername,'staging.invalid');assert.equal(c.ca,'synthetic public CA')}
 }finally{[http.request,https.request]=old}
});
test('invalid origin and TLS failures are not accepted',async()=>{
 await assert.rejects(run({origin:'http://staging.invalid',ca:'public'}));
 await assert.rejects(run({origin:'https://user:password@staging.invalid',ca:'public'}));
 const old=http.request;
 try{http.request=()=>{const r=new EventEmitter();r.end=()=>r.emit('error',Error('synthetic TLS failure'));return r};await assert.rejects(run({origin:'https://staging.invalid',ca:'public'}));}
 finally{http.request=old}
});
