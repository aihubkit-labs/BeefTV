// Browser behavior check with an isolated local fixture; no real platform login.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, mkdtemp, readFile, rm } from "node:fs/promises";
import { resolve } from "node:path";
import { webkit } from "playwright";
import { build as viteBuild } from "vite";
import react from "@vitejs/plugin-react";

const web = resolve(import.meta.dir, "..");
const cache = resolve(web, ".local");
await mkdir(cache, { recursive: true });
const fixture = await mkdtemp(resolve(cache, "aihubkit-brand-"));
let browser, server, page;
try {
    const entry = resolve(fixture, "entry.tsx");
    await Bun.write(entry, `
        import React from 'react';
        import { createRoot } from 'react-dom/client';
        import { App, ConfigProvider } from 'antd';
        import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
        import '@/styles/globals.css';
        import { HubBrandProvider } from '@/integrations/aihubkit/brand';
        import { HubLogin } from '@/integrations/aihubkit/gate';
        import { WorkspaceSidebarSocialLinks } from '@/components/layout/workspace-sidebar-social-links';
        import { WorkspaceSidebarUpdate } from '@/components/layout/workspace-sidebar-update';
        import { ChannelSettingsPane } from '@/pages/settings/channel-settings-pane';
        import AgentsPage from '@/pages/agents';
        import { FullScreenLoader } from '@/components/ui/aceternity/full-screen-loader';
        import { useConfigStore, createModelChannel } from '@/stores/use-config-store';
        import { configureDesktopRuntime } from '@/services/desktop-runtime';
        configureDesktopRuntime({baseURL: location.origin+'/api', launchToken:'fixture-launch'});
        useConfigStore.setState(s => ({config:{...s.config, channels:[createModelChannel({id:'beefapi',name:'BeefTV',pinned:true,credentialRef:'beefapi:managed'})]}}));
        window.runtime = { BrowserOpenURL: url => window.openedURL=url, WindowSetTitle: title => window.nativeTitle=title };
        const collapsed = location.search.includes('collapsed');
        const content = <><HubLogin onLogin={()=>{}} rememberAvailable={false}/><WorkspaceSidebarSocialLinks/><div data-version-fixture style={{width:220}}><div className={'app-workspace-sidebar-utility-row'+(collapsed?' is-collapsed':'')}><WorkspaceSidebarUpdate collapsed={collapsed}/><button className="app-workspace-theme-action">☀</button></div></div><ChannelSettingsPane/><AgentsPage/>{location.search.includes('loading') && <FullScreenLoader/>}</>;
        const brand = !location.search.includes('local');
        createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ConfigProvider><App>{brand?<HubBrandProvider>{content}</HubBrandProvider>:content}</App></ConfigProvider></QueryClientProvider>);
    `);
    const output = resolve(fixture, 'dist');
    await viteBuild({ configFile: false, root: web, publicDir: false, logLevel: 'warn', plugins: [react()], resolve: { alias: { '@': resolve(web, 'src') } }, define: { __APP_VERSION__: '"v1.0.9"', __BEEFTV_HEAVY_MEDIA_ENABLED__: 'true' }, build: { outDir: output, minify: false, rollupOptions: { input: entry, output: { inlineDynamicImports: true, entryFileNames: 'fixture.js', assetFileNames: asset => asset.names.some(name => name.endsWith('.css')) ? 'fixture.css' : 'assets/[name]-[hash].[ext]' } } } });
    const script = await readFile(resolve(output, 'fixture.js'), 'utf8');
    const css = await readFile(resolve(output, 'fixture.css'), 'utf8');
    const loadingCSS = await readFile(resolve(web, 'src/integrations/aihubkit/brand-loading.css'), 'utf8');
    const entryHTML = (await readFile(resolve(web, 'index.html'), 'utf8')).replace('/src/main.tsx', '/fixture.js');
    let platformName = "Customer";
    let failRefresh = false;
    const received = [];
    server = createServer((req, res) => {
        if (req.url.startsWith('/api/integrations/aihubkit/brand')) {
            received.push(req.headers);
            res.setHeader('Content-Type', 'application/json');
            if (failRefresh) { res.writeHead(503); res.end('{}'); return; }
            res.end(JSON.stringify({code:0,data:{id:'customer',origin:'https://brand.example',apiOrigin:'https://api.example',platformName,appName:platformName+'TV',logoUrl:`http://127.0.0.1:${server.address().port}/logo.svg`,bundledLogo:'/logo.svg',websiteUrl:'https://site.example',wechatUrl:'/logo.svg'}}));
        } else if (req.url === '/api/agent-clients') {
            res.setHeader('Content-Type','application/json'); res.end(JSON.stringify({code:0,data:{clients:[],cli:{available:true,path:'fixture',installCommand:'fixture install'}}}));
        } else if (req.url === '/logo.svg' || req.url === '/aihubkit-brand/logo.png') {
            res.setHeader('Content-Type','image/svg+xml'); res.end('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="red"/></svg>');
        } else if (req.url === '/fixture.js') {
            res.setHeader('Content-Type','application/javascript'); res.end(script);
        } else if (req.url === '/fixture.css' || req.url === '/aihubkit-brand/loading.css') {
            res.setHeader('Content-Type','text/css'); res.end(req.url === '/fixture.css' ? css : loadingCSS);
        } else if (req.url.startsWith('/api/')) {
            res.setHeader('Content-Type','application/json'); res.end(JSON.stringify({code:0,data:{state:'disconnected',hasCredential:false,config:{}}}));
        } else {
            res.setHeader('Content-Type','text/html');
            const links = '<link rel="stylesheet" href="/fixture.css">' + (req.url.includes('local') ? '' : '<link rel="stylesheet" href="/aihubkit-brand/loading.css">');
            res.end(entryHTML.replace('</head>', links+'</head>'));
        }
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    browser = await webkit.launch({ headless: true });
    const initial = await browser.newPage({ reducedMotion: 'reduce' });
    await initial.route('**/fixture.js', route => route.fulfill({contentType:'application/javascript',body:''}));
    await initial.goto(`http://127.0.0.1:${server.address().port}`);
    const firstPaint = await initial.locator('#root').evaluate(root => {
        const style = getComputedStyle(root, '::after');
        return {image:style.backgroundImage,mask:style.webkitMaskImage,animation:style.animationName};
    });
    assert.match(firstPaint.image, /aihubkit-brand\/logo.png/);
    assert.equal(firstPaint.mask, 'none');
    assert.equal(firstPaint.animation, 'none');
    await initial.close();
    page = await browser.newPage();
    page.on('console', entry => { if (entry.type() === 'error') console.error('Browser console:',entry.text()); });
    page.on('requestfailed', request => console.error('Browser request:', request.url(), request.failure()));
    const errors = [];
    page.on('pageerror', error => { errors.push(error.message); console.error('Browser error:', error.message); });
    await page.goto(`http://127.0.0.1:${server.address().port}`);
    await page.getByRole('heading', {name:'CustomerTV',exact:true}).waitFor({timeout:10_000});
    assert.equal(await page.getByRole('link', {name:'打开官网'}).count(),1);
    assert.equal(await page.getByRole('link', {name:/GitHub|X 主页/}).count(),0);
    assert.equal(await page.getByRole('button', {name:/捐赠支持/}).count(),0);
    assert.equal(await page.locator('.settings-channel').count(),0);
    assert.equal(await page.locator('.app-workspace-update span').filter({hasText:/CustomerTV-v/}).count(),1);
    assert.equal(await page.getByRole('button',{name:/查看更新日志/}).count(),0);
    assert.equal(await page.getByRole('button',{name:'复制安装命令'}).count(),0);
    await page.getByRole('heading',{name:'CustomerTV MCP',exact:true}).waitFor();
    await page.getByRole('link', {name:'打开官网'}).click();
    assert.equal(await page.evaluate(()=>window.openedURL),'https://site.example');
    await page.getByRole('button', {name:'查看微信二维码'}).click();
    await page.getByRole('img', {name:'微信二维码'}).waitFor();
    platformName = "Renamed";
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    await page.getByRole('heading', {name:'RenamedTV',exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.nativeTitle),'RenamedTV');
    platformName = '很长的客户品牌名称'.repeat(4);
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    const longVersion = page.locator('.app-workspace-update > span[title]').filter({hasText:platformName});
    await longVersion.waitFor();
    const layout = await longVersion.evaluate(element => {
        const [name, version] = element.children;
        const label = element.getBoundingClientRect(), number = version.getBoundingClientRect();
        return {singleLine:getComputedStyle(element).whiteSpace==='nowrap',truncated:name.scrollWidth>name.clientWidth,versionVisible:number.right<=label.right+1,title:element.title};
    });
    assert.equal(layout.singleLine, true);
    assert.equal(layout.truncated, true);
    assert.equal(layout.versionVisible, true);
    assert.equal(layout.title, platformName+'TV-v1.0.9');
    platformName = 'Renamed';
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    await page.getByRole('heading', {name:'RenamedTV',exact:true}).waitFor();
    failRefresh = true;
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    assert.equal(await page.getByRole('heading', {name:'RenamedTV',exact:true}).count(),1);
    assert.ok(received.every(headers=>headers['x-desktop-token']==='fixture-launch'));
    failRefresh = false;
    await page.goto(`http://127.0.0.1:${server.address().port}?loading&collapsed`);
    await page.getByRole('heading', {name:'RenamedTV',exact:true}).waitFor();
    assert.equal(await page.locator('.app-workspace-update > span[title]').count(),0);
    const loader = page.locator('[data-full-screen-loader] .beeftv-loading-logo');
    const loading = await loader.evaluate(element => ({image:getComputedStyle(element).backgroundImage,mask:getComputedStyle(element).webkitMaskImage}));
    assert.match(loading.image, /logo.svg/);
    assert.equal(loading.mask, 'none');
    await page.goto(`http://127.0.0.1:${server.address().port}?local`);
    await page.getByRole('link',{name:'打开 BeefTV 官网'}).waitFor();
    assert.equal(await page.locator('.settings-channel').count(),1);
    assert.equal(await page.getByRole('button',{name:/查看更新日志/}).count(),1);
    await page.locator('.agent-cli-details').waitFor();
    assert.deepEqual(errors,[]);
    console.log('WebKit: first-paint/loading logos, long/collapsed version, CLI visibility, brand refresh, contacts and local mode passed.');
} catch (error) {
    if (page) {
        console.error('Fixture page:', await page.locator('body').innerText());
        console.error('Fixture scripts:', await page.locator('script').evaluateAll(scripts => scripts.map(script => ({src:script.src,type:script.type}))));
        await page.screenshot({path:resolve(cache,'aihubkit-brand-failure.png'),fullPage:true});
    }
    throw error;
} finally {
    await browser?.close();
    server?.closeAllConnections(); server?.close();
    await rm(fixture,{recursive:true,force:true});
}
