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
let browser, server;
try {
    const entry = resolve(fixture, "entry.tsx");
    await Bun.write(entry, `
        import React from 'react';
        import { createRoot } from 'react-dom/client';
        import { App, ConfigProvider } from 'antd';
        import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
        import { HubBrandProvider } from '@/integrations/aihubkit/brand';
        import { HubLogin } from '@/integrations/aihubkit/gate';
        import { WorkspaceSidebarSocialLinks } from '@/components/layout/workspace-sidebar-social-links';
        import { WorkspaceSidebarUpdate } from '@/components/layout/workspace-sidebar-update';
        import { ChannelSettingsPane } from '@/pages/settings/channel-settings-pane';
        import { useConfigStore, createModelChannel } from '@/stores/use-config-store';
        import { configureDesktopRuntime } from '@/services/desktop-runtime';
        configureDesktopRuntime({baseURL: location.origin+'/api', launchToken:'fixture-launch'});
        useConfigStore.setState(s => ({config:{...s.config, channels:[createModelChannel({id:'beefapi',name:'BeefTV',pinned:true,credentialRef:'beefapi:managed'})]}}));
        window.runtime = { BrowserOpenURL: url => window.openedURL=url, WindowSetTitle: title => window.nativeTitle=title };
        const content = <><HubLogin onLogin={()=>{}} rememberAvailable={false}/><WorkspaceSidebarSocialLinks/><WorkspaceSidebarUpdate collapsed={false}/><ChannelSettingsPane/></>;
        const brand = !location.search.includes('local');
        createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><ConfigProvider><App>{brand?<HubBrandProvider>{content}</HubBrandProvider>:content}</App></ConfigProvider></QueryClientProvider>);
    `);
    const output = resolve(fixture, 'dist');
    await viteBuild({ configFile: false, root: web, publicDir: false, logLevel: 'warn', plugins: [react()], resolve: { alias: { '@': resolve(web, 'src') } }, define: { __APP_VERSION__: '"v1.0.9"', __BEEFTV_HEAVY_MEDIA_ENABLED__: 'true' }, build: { outDir: output, minify: false, rollupOptions: { input: entry, output: { inlineDynamicImports: true, entryFileNames: 'fixture.js' } } } });
    const script = await readFile(resolve(output, 'fixture.js'));
    let platformName = "Customer";
    let failRefresh = false;
    const received = [];
    server = createServer((req, res) => {
        if (req.url.startsWith('/api/integrations/aihubkit/brand')) {
            received.push(req.headers);
            res.setHeader('Content-Type', 'application/json');
            if (failRefresh) { res.writeHead(503); res.end('{}'); return; }
            res.end(JSON.stringify({code:0,data:{id:'customer',origin:'https://brand.example',apiOrigin:'https://api.example',platformName,appName:platformName+'TV',logoUrl:`http://127.0.0.1:${server.address().port}/logo.svg`,bundledLogo:'/logo.svg',websiteUrl:'https://site.example',wechatUrl:'/logo.svg'}}));
        } else if (req.url === '/logo.svg') {
            res.setHeader('Content-Type','image/svg+xml'); res.end('<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="red"/></svg>');
        } else if (req.url === '/fixture.js') {
            res.setHeader('Content-Type','application/javascript'); res.end(script);
        } else if (req.url.startsWith('/api/')) {
            res.setHeader('Content-Type','application/json'); res.end(JSON.stringify({code:0,data:{state:'disconnected',hasCredential:false,config:{}}}));
        } else { res.setHeader('Content-Type','text/html'); res.end('<div id="root"></div><script type="module" src="/fixture.js"></script>'); }
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    browser = await webkit.launch({ headless: true });
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', error => { errors.push(error.message); console.error('Browser error:', error.message); });
    await page.goto(`http://127.0.0.1:${server.address().port}`);
    await page.getByRole('heading', {name:'CustomerTV',exact:true}).waitFor();
    assert.equal(await page.getByRole('link', {name:'打开官网'}).count(),1);
    assert.equal(await page.getByRole('link', {name:/GitHub|X 主页/}).count(),0);
    assert.equal(await page.getByRole('button', {name:/捐赠支持/}).count(),0);
    assert.equal(await page.locator('.settings-channel').count(),0);
    assert.equal(await page.locator('.app-workspace-update span').filter({hasText:/CustomerTV-v/}).count(),1);
    assert.equal(await page.getByRole('button',{name:/查看更新日志/}).count(),0);
    await page.getByRole('link', {name:'打开官网'}).click();
    assert.equal(await page.evaluate(()=>window.openedURL),'https://site.example');
    await page.getByRole('button', {name:'查看微信二维码'}).click();
    await page.getByRole('img', {name:'微信二维码'}).waitFor();
    platformName = "Renamed";
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    await page.getByRole('heading', {name:'RenamedTV',exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.nativeTitle),'RenamedTV');
    failRefresh = true;
    await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
    assert.equal(await page.getByRole('heading', {name:'RenamedTV',exact:true}).count(),1);
    assert.ok(received.every(headers=>headers['x-desktop-token']==='fixture-launch'));
    await page.goto(`http://127.0.0.1:${server.address().port}?local`);
    await page.getByRole('link',{name:'打开 BeefTV 官网'}).waitFor();
    assert.equal(await page.locator('.settings-channel').count(),1);
    assert.equal(await page.getByRole('button',{name:/查看更新日志/}).count(),1);
    assert.deepEqual(errors,[]);
    console.log('WebKit: prelogin brand, refresh, QR, website, passive version, hidden builtin channel and local mode passed.');
} finally {
    await browser?.close();
    server?.closeAllConnections(); server?.close();
    await rm(fixture,{recursive:true,force:true});
}
