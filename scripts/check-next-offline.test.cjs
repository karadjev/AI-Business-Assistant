const {test}=require('node:test');const assert=require('node:assert/strict');const {prerequisites}=require('./check-next-offline.cjs');
function fixture(opts={},failure='',version='1.0.0'){
 const calls=[];const result=prerequisites({...opts,resolve:p=>{calls.push(p);if(p===failure)throw Error('absent');return p},load:p=>{if(p===failure)throw Error('absent');return {version:p.startsWith('next/')||p==='@tailwindcss/oxide/package.json'||p==='lightningcss/package.json'?'1.0.0':version}}});return {result,calls};
}
test('missing native binding blocks instead of exit-zero success',()=>{assert.equal(fixture({platform:'linux',arch:'x64',libc:'gnu'},'@next/swc-linux-x64-gnu/package.json').result.status,'BLOCKED')});
test('checks all three exact native versions and loading',()=>{const {result,calls}=fixture({platform:'linux',arch:'x64',libc:'gnu'});assert.equal(result.status,'PREREQUISITES_ONLY');assert.equal(calls.length,3);assert.ok(calls.includes('lightningcss-linux-x64-gnu'))});
test('wrong package version blocks',()=>{assert.equal(fixture({platform:'darwin',arch:'x64'},'', '0.9.0').result.status,'BLOCKED')});
test('musl is not mistaken for glibc',()=>{assert.equal(fixture({platform:'linux',arch:'arm64',libc:'musl'}).result.packages[0].name,'@next/swc-linux-arm64-musl')});
test('unsupported runner blocks without loading',()=>{const {result,calls}=fixture({platform:'other',arch:'x64'});assert.equal(result.status,'BLOCKED');assert.equal(calls.length,0)});
