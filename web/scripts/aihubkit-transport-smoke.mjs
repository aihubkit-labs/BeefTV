// Run with Bun after `bunx playwright install webkit`.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { resolve } from "node:path";
import { webkit } from "playwright";

const web = resolve(import.meta.dir, "..");
const cache = resolve(web, "../.local/cache");
await mkdir(cache, { recursive: true });
const fixture = await mkdtemp(resolve(cache, "aihubkit-transport-"));
let browser;
let server;
const streams = new Set();
try {
    const entry = resolve(fixture, "entry.ts");
    await Bun.write(entry, `
        import { installHubFetch, setHubSession } from '@/integrations/aihubkit/api';
        import { configureDesktopRuntime } from '@/services/desktop-runtime';
        configureDesktopRuntime({baseURL: location.origin + '/api', launchToken: 'fixture-launch'});
        setHubSession('fixture-session');
        installHubFetch();
        window.clearSession = () => setHubSession();
        window.beginStream = async (useRequest) => {
            const controller = new AbortController();
            window.abortStream = () => controller.abort();
            const url = location.origin + '/api/events';
            const init = {signal:controller.signal, headers:{Accept:'text/event-stream'}};
            const response = await (useRequest ? fetch(new Request(url, init)) : fetch(url, init));
            const reader = response.body.getReader();
            await reader.read();
            window.readDone = reader.read().then(() => 'ended', error => error.name);
        };
        window.ready = true;
    `);
    const build = await Bun.build({ entrypoints: [entry], target: "browser", format: "esm", define: { "import.meta.env": "{}" }, plugins: [{ name: "workspace-alias", setup(builder) {
        builder.onResolve({ filter: /^@\// }, args => ({ path: Bun.resolveSync("./src/" + args.path.slice(2), web) }));
    } }] });
    assert.ok(build.success, build.logs.join("\n"));
    const script = await build.outputs[0].text();
    let opened = 0;
    let closed = 0;
    const received = [];
    server = createServer(async (req, res) => {
        if (req.url === "/api/events") {
            opened++;
            streams.add(res);
            received.push(req.headers);
            res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" });
            res.write("event: ready\ndata: {}\n\n");
            res.on("close", () => { closed++; streams.delete(res); });
        } else if (req.url === "/api/save" || req.url === "/foreign") {
            let body = "";
            for await (const chunk of req) body += chunk;
            res.writeHead(200, { "Content-Type": "application/json" });
            res.end(JSON.stringify({ body, headers: req.headers }));
        } else if (req.url === "/fixture.js") {
            res.writeHead(200, { "Content-Type": "application/javascript" });
            res.end(script);
        } else {
            res.writeHead(200, { "Content-Type": "text/html" });
            res.end('<script type="module" src="/fixture.js"></script>');
        }
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    const base = `http://127.0.0.1:${server.address().port}`;
    browser = await webkit.launch({ headless: true });
    const page = await browser.newPage();
    await page.goto(base);
    await page.waitForFunction(() => window.ready);
    // Exceed HTTP/1.1's six connections. GC exposes lost dependent abort
    // signals in WebKit; a simple immediate abort misses this regression.
    for (let i = 0; i < 12; i++) {
        await page.evaluate(useRequest => window.beginStream(useRequest), i % 2 === 1);
        await page.requestGC();
        await page.evaluate(() => window.abortStream());
        const outcome = await page.evaluate(() => Promise.race([
            window.readDone, new Promise(resolve => setTimeout(() => resolve("still open"), 2000)),
        ]));
        assert.equal(outcome, "AbortError", `stream ${i}, Request input: ${i % 2 === 1}`);
        const saved = await page.evaluate(async () => {
            const response = await fetch("/api/save", { method: "POST", body: "canvas draft", credentials: "include", signal: AbortSignal.timeout(2000) });
            return response.json();
        });
        assert.equal(saved.body, "canvas draft");
        assert.equal(saved.headers["x-desktop-token"], "fixture-launch");
        assert.equal(saved.headers["x-aihubkit-session"], "fixture-session");
    }
    const foreign = await page.evaluate(async () => (await fetch("/foreign")).json());
    assert.equal(foreign.headers["x-desktop-token"], undefined);
    assert.equal(foreign.headers["x-aihubkit-session"], undefined);
    await page.evaluate(() => window.clearSession());
    const local = await page.evaluate(async () => (await fetch("/api/save")).json());
    assert.equal(local.headers["x-desktop-token"], "fixture-launch");
    assert.equal(local.headers["x-aihubkit-session"], undefined);
    assert.equal(opened, 12);
    assert.equal(closed, 12);
    assert.ok(received.every(headers => headers["x-desktop-token"] === "fixture-launch" && headers["x-aihubkit-session"] === "fixture-session"));
    console.log("WebKit: 12 stream cancellations, subsequent writes, request bodies and credential boundaries passed.");
} finally {
    await browser?.close();
    for (const stream of streams) stream.destroy();
    server?.closeAllConnections();
    server?.close();
    await rm(fixture, { recursive: true, force: true });
}
