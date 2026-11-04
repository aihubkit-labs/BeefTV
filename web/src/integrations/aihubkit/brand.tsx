import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { ProductPresentationContext } from "@/components/brand/product-presentation";
import { commitPublicAppearance, useAppearanceStore } from "@/stores/use-appearance-store";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { WorkspaceErrorState } from "@/components/layout/workspace-state";
import { http } from "@/services/api/request";
import { setModelProtocolPresentation } from "@/lib/model-protocols";
import { useConfigStore } from "@/stores/use-config-store";
import { commitModelConfig } from "@/services/model-config-repository";
import { synchronizeHubModelBrand } from "./catalog";

export type HubBrand = { id: string; origin: string; apiOrigin: string; platformName: string; appName: string; logoUrl: string; websiteUrl: string; wechatUrl: string; bundledLogo?: string };
const fallback: HubBrand = { id: "", origin: "https://apimodel.aihubkit.com", apiOrigin: "https://apimodel.aihubkit.com", platformName: "aihubkit", appName: "BeefTV", logoUrl: "/beef-logo.png", websiteUrl: "", wechatUrl: "" };
const BrandContext = createContext<HubBrand>(fallback);
export const useHubBrand = () => useContext(BrandContext);

type NativePresentation = { BrowserOpenURL?: (url: string) => void; WindowSetTitle?: (title: string) => void };
function native() { return (window as Window & { runtime?: NativePresentation }).runtime; }

async function availableLogo(url: string, fallback: string) {
    return new Promise<string>((resolve) => {
        const probe = new Image();
        const finish = (value: string) => { clearTimeout(timer); probe.onload = null; probe.onerror = null; resolve(value); };
        const timer = window.setTimeout(() => finish(fallback), 8_000);
        probe.onload = () => finish(url);
        probe.onerror = () => finish(fallback);
        probe.src = url;
    });
}

// Mounted only after the canonical model configuration has been hydrated.
export function HubBrandModels({ children }: { children: ReactNode }) {
    const brand = useHubBrand();
    const config = useConfigStore((state) => state.config);
    useEffect(() => {
        const next = synchronizeHubModelBrand(config, brand);
        if (next !== config) commitModelConfig(next);
    }, [brand.apiOrigin, brand.platformName, config]);
    return <>{children}</>;
}

export function HubBrandProvider({ children }: { children: ReactNode }) {
    const [brand, setBrand] = useState<HubBrand>();
    const [failed, setFailed] = useState(false);
    const [attempt, setAttempt] = useState(0);
    useEffect(() => {
        let cancelled = false;
        let pending = false;
        const refresh = async (remote: boolean) => {
            if (pending) return;
            pending = true;
            try {
                const value = await http.get<HubBrand>("/integrations/aihubkit/brand", { params: { refresh: remote ? "1" : "0" } });
                if (cancelled) return;
                if (!value.id) { setBrand(fallback); return; }
                const previous = useAppearanceStore.getState().appearance;
                const logoUrl = remote ? await availableLogo(value.logoUrl, previous.logoUrl) : value.bundledLogo || value.logoUrl;
                if (cancelled) return;
                setBrand(value);
                setModelProtocolPresentation({ "aihubkit-video": `${value.platformName} Video` });
                commitPublicAppearance({ ...previous, brandName: value.appName, brandSlug: value.id, seoTitle: value.appName, seoDescription: `${value.appName}，AI 影视创作工作台。`, footerCopyright: `© ${new Date().getFullYear()} ${value.appName}`, logoUrl, darkLogoUrl: logoUrl, logoConfigured: true, darkLogoConfigured: true, logoFrameEnabled: false, configured: true });
                native()?.WindowSetTitle?.(value.appName);
            } catch { if (!remote && !cancelled) setFailed(true); }
            finally { pending = false; }
        };
        void refresh(false).then(() => refresh(true));
        const onFocus = () => { void refresh(true); };
        const timer = window.setInterval(onFocus, 60_000);
        window.addEventListener("focus", onFocus);
        return () => { cancelled = true; clearInterval(timer); window.removeEventListener("focus", onFocus); setModelProtocolPresentation({}); };
    }, [attempt]);
    if (failed) return <WorkspaceErrorState title="无法读取品牌信息" description="请重试连接本地工作台。" onRetry={() => { setFailed(false); setAttempt((v) => v + 1); }} />;
    if (!brand) return <FullScreenLoader label="正在打开工作台" />;
    const openWebsite = () => { const open = native()?.BrowserOpenURL; if (open) open(brand.websiteUrl); else window.open(brand.websiteUrl, "_blank", "noopener,noreferrer"); };
    return <BrandContext.Provider value={brand}><ProductPresentationContext.Provider value={brand.id ? { contacts: { website: brand.websiteUrl, qrImage: brand.wechatUrl, openWebsite }, staticVersion: true, hideBuiltinChannel: true } : { staticVersion: true, hideBuiltinChannel: true }}>{children}</ProductPresentationContext.Provider></BrandContext.Provider>;
}
