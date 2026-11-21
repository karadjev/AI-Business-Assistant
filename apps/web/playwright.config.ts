import {defineConfig,devices} from "@playwright/test";
export default defineConfig({
 testDir:"./tests",fullyParallel:false,workers:1,reporter:"list",timeout:15000,
 use:{baseURL:process.env.SMOKE_WEB_URL??"http://127.0.0.1:3000",trace:"retain-on-failure",...devices["Desktop Chrome"],launchOptions:{chromiumSandbox:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}},
 projects:[{name:"chromium"}],
});
