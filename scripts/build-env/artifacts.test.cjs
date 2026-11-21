const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),os=require('node:os'),path=require('node:path');const {main,verify}=require('./artifacts.cjs');
test('rejects missing, stale and modified artifacts; accepts only matching fixture proof',async()=>{
 const d=fs.mkdtempSync(path.join(os.tmpdir(),'aba-proof-')),s=path.join(d,'source'),w=path.join(d,'web');fs.mkdirSync(s);fs.writeFileSync(path.join(s,'package.json'),'{}');
 try{await main('copy',w,s);assert.throws(()=>verify(w,s));
 for(const [n,data] of Object.entries({'.next/BUILD_ID':'fixture','.next/standalone/server.js':'fixture','.next/required-server-files.json':'{}','.next/static/fixture/_buildManifest.js':'fixture'})){fs.mkdirSync(path.dirname(path.join(w,n)),{recursive:true});fs.writeFileSync(path.join(w,n),data)}
 await main('record',w,s);verify(w,s);fs.mkdirSync(path.join(s,'tests'));fs.writeFileSync(path.join(s,'tests','browser.spec.ts'),'test-only change');verify(w,s);fs.writeFileSync(path.join(s,'changed.ts'),'new');assert.throws(()=>verify(w,s));fs.unlinkSync(path.join(s,'changed.ts'));
 // The exact E01 paths must remain detectable, even when empty. Never whitelist them.
 for(const name of ['.next 2','node_modules 2']){
  const extra=path.join(w,'.next/standalone',name);fs.mkdirSync(extra);
  assert.throws(()=>verify(w,s),/standalone changed/);
  await assert.rejects(main('record',w,s),/E01 directory present/);
  fs.writeFileSync(path.join(extra,'unexpected.js'),'unexpected runtime bytes');
  assert.throws(()=>verify(w,s),/standalone changed/);
  fs.rmSync(extra,{recursive:true});verify(w,s);
 }
 fs.writeFileSync(path.join(w,'.next/standalone/server.js'),'altered');assert.throws(()=>verify(w,s));
 }finally{fs.rmSync(d,{recursive:true,force:true})}
});
