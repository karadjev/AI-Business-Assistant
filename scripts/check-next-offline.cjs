/* A fail-closed prerequisite check, never a substitute for next build. No network. */
const {createRequire}=require('node:module');
const path=require('node:path');
const fs=require('node:fs');
function metadata(r,name){
 try{return r(name+'/package.json')}catch{
  let dir=path.dirname(r.resolve(name));
  for(;;){const file=path.join(dir,'package.json');if(fs.existsSync(file)){const data=JSON.parse(fs.readFileSync(file,'utf8'));if(data.name===name)return data}
   const parent=path.dirname(dir);if(parent===dir)throw Error('package metadata unavailable');dir=parent;
  }
 }
}
function prerequisites({platform=process.platform,arch=process.arch,libc,resolve,load}) {
 if(platform==='linux'&&!libc)libc=process.report.getReport().header.glibcVersionRuntime?'gnu':'musl';
 if(!['linux','darwin'].includes(platform)||!['x64','arm64'].includes(arch))return {status:'BLOCKED',reason:'unsupported offline runner',packages:[]};
 const suffix=platform==='linux'?`linux-${arch}-${libc}`:`darwin-${arch}`;
 const expected=[['@next/swc-'+suffix,'next/package.json'],['@tailwindcss/oxide-'+suffix,'@tailwindcss/oxide/package.json'],['lightningcss-'+suffix,'lightningcss/package.json']];
 const packages=expected.map(([name,owner])=>{
  let version;
  try{version=load(owner).version;const actual=load(name+'/package.json').version;if(actual!==version)return {name,version,status:'MISMATCH'};resolve(name);load(name);return {name,version,status:'LOADABLE'};}
  catch{return {name,version:version??'UNKNOWN',status:'MISSING_OR_UNLOADABLE'};}
 });
 return {status:packages.every(p=>p.status==='LOADABLE')?'PREREQUISITES_ONLY':'BLOCKED',platform,arch,libc,packages};
}
if(require.main===module){
 const root=process.env.ABA_WEB_ROOT??path.resolve(__dirname,'../apps/web');
 const r=createRequire(path.join(root,'package.json'));
 const report=prerequisites({resolve:p=>r.resolve(p),load:p=>p.endsWith('/package.json')?metadata(r,p.slice(0,-13)):r(p)});
 console.log(JSON.stringify(report,null,2));
 console.log('This checks native prerequisites only. PASS requires a fresh production build and its browser tests. No download attempted.');
 if(report.status==='BLOCKED')process.exitCode=42;
}
module.exports={prerequisites};
