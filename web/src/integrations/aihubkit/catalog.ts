import { protocolForBeefAPICatalog, type ModelProtocolDefinition } from "@/lib/model-protocols";
import { normalizeConfigSnapshot, type AiConfig, type ModelChannel } from "@/stores/use-config-store";
import type { HubModel } from "./api";

export function synchronizeHubModelBrand(config: AiConfig, brand: { apiOrigin: string; platformName: string }): AiConfig {
    let changed = false;
    const channels = config.channels.map((channel) => {
        if (!channel.credentialRef?.startsWith("aihubkit:")) return channel;
        const name = `${brand.platformName} · ${channel.credentialRef.slice("aihubkit:".length)}`;
        if (channel.baseUrl === brand.apiOrigin && channel.name === name) return channel;
        changed = true;
        return { ...channel, baseUrl: brand.apiOrigin, name };
    });
    return changed ? { ...config, channels } : config;
}

export function hubModelProtocol(model: HubModel) {
    const endpoints = (model.supported_endpoint_types || []).map((value) => value.toLowerCase());
    if (endpoints.includes("openai-video") || endpoints.includes("aihubkit-video")) return "aihubkit-video";
    return protocolForBeefAPICatalog(endpoints) || (endpoints.includes("openai") ? "chat-completion" : undefined);
}

export function importedHubModels(config: AiConfig) {
    return config.channels.filter((channel) => channel.credentialRef?.startsWith("aihubkit:")).flatMap((channel) =>
        channel.models.map((model) => {
            const profile = channel.modelProfiles?.find((item) => item.model === model);
            const id = `${channel.id}::${model}`;
            return {
                id, model, group: channel.credentialRef!.slice("aihubkit:".length),
                protocol: profile?.protocol || channel.interfaceType,
                defaults: (["text", "image", "video", "audio"] as const).filter((capability) => config[`${capability}Model`] === id),
            };
        }),
    );
}

export function resetHubModels(config: AiConfig, defaults: Partial<AiConfig>): AiConfig {
    const removed = new Set(config.channels.filter((channel) => channel.credentialRef?.startsWith("aihubkit:")).map((channel) => channel.id));
    const channels = config.channels.filter((channel) => !removed.has(channel.id));
    const incoming = defaults.channels || [];
    if (incoming.some((channel) => channels.some((existing) => existing.id === channel.id))) {
        throw new Error("默认渠道与已有独立渠道 ID 冲突，请先在模型配置中调整该独立渠道");
    }
    const next = { ...config };
    for (const field of ["model", "textModel", "imageModel", "videoModel", "audioModel", "assistantModel", "mediaReviewModel"] as const) {
        if (removed.has((next[field] || "").split("::")[0]!)) next[field] = "";
    }
    return normalizeConfigSnapshot({ config: { ...next, ...defaults, channels: [...channels, ...incoming] } }).config;
}

// Import is additive: a refresh cannot discard custom models, protocol choices
// or the user's existing defaults. Unknown contracts require an explicit choice.
export function importHubModels(config: AiConfig, group: string, models: HubModel[], overrides: Record<string, string>, definitions: ModelProtocolDefinition[], brand: { apiOrigin: string; platformName: string }): AiConfig {
    if (!group || models.length === 0) throw new Error("请选择分组和需要导入的模型");
    const id = "aihubkit-" + group;
    const previous = config.channels.find((channel) => channel.id === id);
    const profiles = new Map((previous?.modelProfiles || []).map((profile) => [profile.model, profile]));
    for (const model of models) {
        const protocol = overrides[model.id] || profiles.get(model.id)?.protocol || hubModelProtocol(model);
        const definition = definitions.find((item) => item.value === protocol && item.enabled !== false);
        if (!definition) throw new Error(`请为 ${model.id} 选择已安装的协议`);
        profiles.set(model.id, { ...profiles.get(model.id), model: model.id, capability: definition.capability, protocol });
    }
    const channel: ModelChannel = {
        ...previous, id, name: previous?.name || `${brand.platformName} · ${group}`, scope: "user", enabled: true,
        // Each model owns its protocol; a channel-wide text fallback masks plugin capabilities.
        pinned: true, apiFormat: "openai", interfaceType: undefined, baseUrl: brand.apiOrigin,
        apiKey: "", secretKey: "", headers: [], hasApiKey: true, credentialRef: "aihubkit:" + group,
        models: [...new Set([...(previous?.models || []), ...models.map((model) => model.id)])],
        modelProfiles: [...profiles.values()],
    };
    const next = { ...config, channels: [...config.channels.filter((item) => item.id !== id), channel] };
    for (const profile of channel.modelProfiles || []) {
        const field = `${profile.capability}Model` as "textModel" | "imageModel" | "videoModel" | "audioModel";
        if (!next[field]) next[field] = `${id}::${profile.model}`;
    }
    return normalizeConfigSnapshot({ config: next }).config;
}
