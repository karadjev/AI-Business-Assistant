// Always verify the packaged tree before loading Next, including on restart.
const {spawnSync}=require('node:child_process');
const result=spawnSync(process.execPath,['/guard/artifacts.cjs','verify'],{
 env:{...process.env,ABA_WEB_ROOT:'/apps/web',ABA_WEB_SOURCE:'/apps/web'},stdio:'inherit',
});
if(result.error||result.status!==0)process.exit(42);
require('/apps/web/.next/standalone/server.js');
