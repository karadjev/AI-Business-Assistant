/* Private network only. No credentials, response bodies, cookies or URLs logged. */
const http=require('node:http'),https=require('node:https'),fs=require('node:fs');
function request(client,options){return new Promise((resolve,reject)=>{
 const req=client.request({...options,timeout:5000},res=>{res.resume();resolve(res.statusCode)});
 req.on('timeout',()=>req.destroy());req.on('error',reject);req.end();
})}
async function run(config){
 const origin=new URL(config.origin);
 if(origin.protocol!=='https:'||origin.username||origin.password||origin.pathname!=='/'||origin.search||origin.hash)throw Error('invalid origin');
 const tls={hostname:'proxy',port:8443,servername:origin.hostname,ca:config.ca,rejectUnauthorized:true,headers:{Host:origin.host}};
 const result={
  api_ready:await request(http,{hostname:'api',port:8080,path:'/health/ready'}),
  tls_login:await request(https,{...tls,path:'/login'}),
  unauthenticated:await request(https,{...tls,path:'/api/v1/auth/me'}),
  csrf:await request(https,{...tls,path:'/api/v1/organizations',method:'POST'}),
 };
 if(JSON.stringify(result)!==JSON.stringify({api_ready:200,tls_login:200,unauthenticated:401,csrf:403}))throw Error('unexpected status');
 return result;
}
// The coordinator evaluates this source with node -e, where require.main is
// undefined. module.parent also handles direct-file execution and safe imports.
if(!module.parent)run(JSON.parse(fs.readFileSync(0,'utf8'))).then(r=>console.log(JSON.stringify(r))).catch(()=>{console.error('private smoke failed');process.exitCode=1});
module.exports={run};
