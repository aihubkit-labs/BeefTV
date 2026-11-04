import { apiBaseURL, apiClient, http } from "@/services/api/request";
import type { AiConfig } from "@/stores/use-config-store";

export type HubUser = { id: number; username: string; display_name: string; group: string; quota: number; used_quota: number };
export type HubSession = { enabled: boolean; authenticated: boolean; rememberLoginAvailable?: boolean; uploadToAIHubkit?: boolean; session?: string; scope?: string; user?: HubUser; groups?: Record<string, { desc: string; ratio: number }>; modelImportWarnings?: string[] };
export type HubLoginForm = { username: string; password: string };
export type HubModel = { id: string; model_type?: string; supported_endpoint_types?: string[] };
export type UsagePage = { page: number; page_size: number; total: number; items: Array<{ id: number; created_at: number; model_name: string; quota: number; group: string; prompt_tokens: number; completion_tokens: number }> };

const prefix = "/integrations/aihubkit/";
let session = "";
export function setHubSession(value?: string) { session = value || ""; }
export function hubSessionHeaders(): Record<string, string> { return session ? { "X-Aihubkit-Session": session } : {}; }

export function isBackendURL(value: string, baseURL: string, locationURL: string): boolean {
    const base = new URL(baseURL.replace(/\/$/u, "") + "/", locationURL);
    const target = new URL(value, locationURL);
    return target.origin === base.origin && target.pathname.startsWith(base.pathname);
}

// This uses the official business client. The session stays in memory and is
// attached only to this backend; remote inference/storage credentials never
// reach the browser.
apiClient.interceptors.request.use((config) => {
    if (session && isBackendURL(apiClient.getUri(config), apiBaseURL, window.location.href)) config.headers.set("X-Aihubkit-Session", session);
    return config;
});

// This module loads before desktop bootstrap via custom-channel-relay. Keep
// the original transport: chained Request copies can lose abort propagation
// after WebKit GC, leaving SSE connections open and blocking canvas saves.
const originalFetch = globalThis.fetch.bind(globalThis);

export function installHubFetch() {
    const previousFetch = globalThis.fetch;
    const hubFetch: typeof fetch = (input, init) => {
        const target = input instanceof Request ? input.url : new URL(String(input), window.location.href).href;
        if (!isBackendURL(target, apiBaseURL, window.location.href)) return originalFetch(input, init);
        const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
        if (session) headers.set("X-Aihubkit-Session", session);
        const launchToken = apiClient.defaults.headers.common["X-Desktop-Token"];
        if (typeof launchToken === "string" && launchToken) headers.set("X-Desktop-Token", launchToken);
        // Preserve the caller's signal, body, credentials and other options.
        return originalFetch(input instanceof Request ? input : target, {
            ...init, headers, signal: init?.signal !== undefined ? init.signal : (input instanceof Request ? input.signal : undefined),
        });
    };
    globalThis.fetch = hubFetch;
    return () => { if (globalThis.fetch === hubFetch) globalThis.fetch = previousFetch; };
}

export const hub = {
    session: () => http.get<HubSession>(prefix + "session"),
    login: (username: string, password: string, rememberCredentials = false) => http.post<HubSession>(prefix + "login", { username, password, rememberCredentials }, { timeout: 120_000, headers: { "X-Aihubkit-Client": "1" } }),
    loginForm: () => http.post<HubLoginForm | null>(prefix + "login-form", {}, { headers: { "X-Aihubkit-Client": "1" } }),
    forgetLoginForm: () => http.delete<boolean>(prefix + "login-form", { headers: { "X-Aihubkit-Client": "1" } }),
    register: (values: { username: string; password: string; email: string; verification_code?: string; aff_code?: string }) => http.post<boolean>(prefix + "register", values, { timeout: 60_000, headers: { "X-Aihubkit-Client": "1" } }),
    verification: (email: string) => http.post<boolean>(prefix + "verification", { email }, { timeout: 60_000, headers: { "X-Aihubkit-Client": "1" } }),
    self: () => http.get<HubUser>(prefix + "self", { timeout: 60_000 }),
    catalog: (group: string) => http.post<HubModel[]>(prefix + "catalog", { group }, { timeout: 60_000 }),
    defaultModelConfig: () => http.post<{ config: Partial<AiConfig>; warnings: string[] }>(prefix + "default-model-config", {}, { timeout: 120_000 }),
    logs: (page = 1) => http.get<UsagePage>(prefix + "logs", { params: { p: page, page_size: 10 }, timeout: 60_000 }),
    topup: (key: string) => http.post(prefix + "topup", { key }, { timeout: 60_000 }),
    logout: () => http.post(prefix + "logout", {}, { timeout: 660_000 }),
    setUploadPreference: (uploadToAIHubkit: boolean) => http.post<HubSession>(prefix + "upload-preference", { uploadToAIHubkit }),
};
