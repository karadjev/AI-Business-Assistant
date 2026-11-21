/* Local artifact provenance gate. Never downloads or installs. */
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const skip=new Set(['node_modules','.next','.git','test-results','playwright-report']);
const excluded=n=>skip.has(n)||n.startsWith('.env')||n.endsWith('.tsbuildinfo')||n==='next-env.d.ts';
function digest(dir){
 const hash=crypto.createHash('sha256');
 function walk(d,base=''){for(const name of fs.readdirSync(d).sort()){if(excluded(name)||base===''&&(name==='tests'||name==='playwright.config.ts'))continue;const p=path.join(d,name),r=path.join(base,name),s=fs.lstatSync(p);if(s.isSymbolicLink())throw Error('symlink in source/artifact');if(s.isDirectory())walk(p,r);else{hash.update(r+'\0');hash.update(fs.readFileSync(p));hash.update('\0')}}}walk(dir);return hash.digest('hex');
}
const sha=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
function assertNoE01(web){for(const n of ['.next 2','node_modules 2'])if(fs.existsSync(path.join(web,'.next/standalone',n)))throw Error('standalone changed: E01 directory present')}
function verify(web,source){
 assertNoE01(web);
 const m=JSON.parse(fs.readFileSync(path.join(web,'.next/aba-build-proof.json'),'utf8'));
 if(m.schema!==1||m.source!==digest(source)||m.source!==digest(web))throw Error('source snapshot changed or stale build proof');
 if(m.buildID!==fs.readFileSync(path.join(web,'.next/BUILD_ID'),'utf8').trim())throw Error('BUILD_ID mismatch');
 for(const [n,v] of Object.entries(m.files)){if(sha(path.join(web,n))!==v)throw Error('artifact changed')}
 if(m.standalone!==standaloneDigest(web))throw Error('standalone changed');
 return m;
}
// Hash runtime tree including node_modules: source exclusions must not hide runtime edits.
function standaloneDigest(web){const h=crypto.createHash('sha256');function walk(d,b=''){for(const n of fs.readdirSync(d).sort()){const p=path.join(d,n),r=path.join(b,n),s=fs.lstatSync(p);h.update(r+'\0');if(s.isSymbolicLink())h.update('link:'+fs.readlinkSync(p));else if(s.isDirectory())walk(p,r);else h.update(fs.readFileSync(p));h.update('\0')}}walk(path.join(web,'.next/standalone'));return h.digest('hex')}
async function main(op,web,source){
 if(op==='copy'){if(path.resolve(web)===path.resolve(source))throw Error('workspace must differ from source');fs.mkdirSync(web,{recursive:true});
  // Refuse extra stale source files instead of silently compiling an old workspace.
  for(const n of fs.readdirSync(web)){if(!excluded(n)&&!fs.existsSync(path.join(source,n)))throw Error('stale workspace file; create a fresh dedicated workspace')}
  fs.cpSync(source,web,{recursive:true,filter:p=>!path.relative(source,p).split(path.sep).some(excluded)});console.log('Copied source only; no env/native modules');return;
 }
 if(op==='record'){
  assertNoE01(web);
  if(digest(web)!==digest(source))throw Error('source changed during build');
  const names=['.next/BUILD_ID','.next/standalone/server.js','.next/required-server-files.json'];
  const buildID=fs.readFileSync(path.join(web,names[0]),'utf8').trim();if(!/^[A-Za-z0-9_-]+$/.test(buildID))throw Error('invalid BUILD_ID');
  const marker=`.next/static/${buildID}/_buildManifest.js`;names.push(marker);
  const m={schema:1,createdAt:new Date().toISOString(),source:digest(source),buildID,standalone:standaloneDigest(web),files:Object.fromEntries(names.map(n=>[n,sha(path.join(web,n))]))};
  fs.writeFileSync(path.join(web,'.next/aba-build-proof.json'),JSON.stringify(m,null,2));console.log('Build proof recorded; browser E2E has not run');return;
 }
 const m=verify(web,source);
 if(op==='served'){
  const u=new URL(process.env.SMOKE_WEB_URL??'http://web:3000');
  if(u.protocol!=='http:'||!['web','127.0.0.1','localhost'].includes(u.hostname)||u.username||u.password)throw Error('only local test web origin allowed');
  const n=`.next/static/${m.buildID}/_buildManifest.js`;
  const r=await fetch(new URL('/_next/static/'+m.buildID+'/_buildManifest.js',u),{redirect:'error',signal:AbortSignal.timeout(5000)});
  if(!r.ok||crypto.createHash('sha256').update(Buffer.from(await r.arrayBuffer())).digest('hex')!==m.files[n])throw Error('served build does not match artifact');
 }
 console.log('VERIFIED: current artifact'+(op==='served'?' served by local web':''));
}
if(require.main===module)main(process.argv[2],process.env.ABA_WEB_ROOT??'/apps/web',process.env.ABA_WEB_SOURCE??'/web-source').catch(()=>{console.error('BLOCKED: missing/stale artifact, source mismatch, or local build unavailable; no browser tests started');process.exitCode=42});
module.exports={digest,verify,main};
