import { expect, test } from "bun:test";

import { importedHubModels, importHubModels, resetHubModels } from "../src/integrations/aihubkit/catalog";
import type { ModelProtocolDefinition } from "../src/lib/model-protocols";
import {
    createModelChannel,
    defaultConfig,
    filterModelsByCapability,
    normalizeConfigSnapshot,
    resolveModelRequestConfig,
} from "../src/stores/use-config-store";

const definitions: ModelProtocolDefinition[] = [
    { value: "chat-completion", capability: "text", label: "Chat", create: "POST /v1/chat/completions", contentType: "application/json", media: "" },
    { value: "openai-image", capability: "image", label: "Image", create: "POST /v1/images/generations", contentType: "application/json", media: "" },
    { value: "aihubkit-video", capability: "video", label: "AIHubKit Video", create: "POST /v1/videos", contentType: "application/json", media: "" },
    { value: "openai-audio", capability: "audio", label: "Audio", create: "POST /v1/audio/speech", contentType: "application/json", media: "" },
];

test.each(["catalog", "manual"] as const)("%s video protocol imports are selectable immediately and after reload", (selection) => {
    const config = normalizeConfigSnapshot({ config: defaultConfig }).config;
    const imported = importHubModels(config, "video", [{
        id: "sd-mini-z",
        supported_endpoint_types: [selection === "catalog" ? "openai-video" : "openai"],
    }], selection === "manual" ? { "sd-mini-z": "aihubkit-video" } : {}, definitions);
    const value = "aihubkit-video::sd-mini-z";

    for (const current of [imported, normalizeConfigSnapshot({ config: JSON.parse(JSON.stringify(imported)) }).config]) {
        expect(filterModelsByCapability(current.models, "video", current.channels)).toEqual([value]);
        expect(filterModelsByCapability(current.models, "text", current.channels)).toEqual([]);
        expect(current.videoModels).toEqual([value]);
        expect(current.videoModel).toBe(value);
        expect(resolveModelRequestConfig(current, value)).toMatchObject({
            model: "sd-mini-z", interfaceType: "aihubkit-video", credentialRef: "aihubkit:video", apiKey: "",
        });
    }
    expect(config.channels).toEqual([]);
    expect(config.models).toEqual([]);
});

test("initialization removes all imported groups and restores three defaults while keeping independent channels", () => {
    const custom = createModelChannel({ id: "custom", apiKey: "synthetic-key", models: ["chosen"], modelProfiles: [{ model: "chosen", capability: "text", protocol: "openai-response" }] });
    const old = importHubModels({ ...defaultConfig, channels: [custom], textModel: "custom::chosen" }, "old", [{ id: "old-model" }], { "old-model": "openai-audio" }, definitions);
    old.assistantModel = "aihubkit-old::old-model";
    old.mediaReviewModel = "aihubkit-old::old-model";
    const defaults = {
        aihubkitDefaultsInitialized: true,
        channels: [
            createModelChannel({ id: "aihubkit-default-企业版", credentialRef: "aihubkit:default-企业版", models: ["gpt-5.10"], modelProfiles: [{ model: "gpt-5.10", capability: "text", protocol: "chat-completion" }] }),
            createModelChannel({ id: "aihubkit-gpt_image_web", credentialRef: "aihubkit:gpt_image_web", models: ["gpt-image-2-w"], modelProfiles: [{ model: "gpt-image-2-w", capability: "image", protocol: "openai-image" }] }),
            createModelChannel({ id: "aihubkit-video_官key", credentialRef: "aihubkit:video_官key", models: ["sd-mini-z"], modelProfiles: [{ model: "sd-mini-z", capability: "video", protocol: "aihubkit-video" }] }),
        ],
        textModel: "aihubkit-default-企业版::gpt-5.10", imageModel: "aihubkit-gpt_image_web::gpt-image-2-w", videoModel: "aihubkit-video_官key::sd-mini-z",
    };
    const reset = resetHubModels(old, defaults);
    for (const config of [reset, resetHubModels(reset, defaults), normalizeConfigSnapshot({ config: JSON.parse(JSON.stringify(reset)) }).config]) {
        expect(config.channels).toHaveLength(4);
        expect(config.channels[0]).toEqual(custom);
        expect(importedHubModels(config).map((row) => row.defaults)).toEqual([["text"], ["image"], ["video"]]);
        expect(config.models.some((id) => id.startsWith("aihubkit-old::"))).toBe(false);
        expect(config.audioModel).toBe("");
        expect(config.assistantModel).toBe("");
        expect(config.mediaReviewModel).toBe("");
        expect(resolveModelRequestConfig(config, config.videoModel).interfaceType).toBe("aihubkit-video");
    }
    expect(old.channels.some((channel) => channel.id === "aihubkit-old")).toBe(true);
    const skipped = resetHubModels(old, { channels: [] });
    expect(importedHubModels(skipped)).toEqual([]);
    expect(skipped.textModel).toBe("custom::chosen");
    expect(skipped.audioModel).toBe("");
});

test("one imported group exposes each model under its own capability", () => {
    const imported = importHubModels(defaultConfig, "mixed", definitions.map((definition, index) => ({ id: `opaque-${index}` })),
        Object.fromEntries(definitions.map((definition, index) => [`opaque-${index}`, definition.value])), definitions);

    for (const [index, definition] of definitions.entries()) {
        const value = `aihubkit-mixed::opaque-${index}`;
        expect(filterModelsByCapability(imported.models, definition.capability, imported.channels)).toEqual([value]);
        expect(imported[`${definition.capability}Models`]).toEqual([value]);
        expect(imported[`${definition.capability}Model`]).toBe(value);
    }
});

test("reimport repairs a channel with a text fallback while preserving defaults and model settings", () => {
    const custom = createModelChannel({
        id: "custom", models: ["existing"], apiKey: "synthetic-key",
        modelProfiles: [{ model: "existing", capability: "video", protocol: "newapi" }],
    });
    const profile = {
        model: "opaque", capability: "video" as const, protocol: "aihubkit-video", displayName: "My model",
        referenceMediaTransport: { image: "url" as const },
    };
    const previous = createModelChannel({
        id: "aihubkit-video", name: "My group", interfaceType: "chat-completion", credentialRef: "aihubkit:video",
        models: ["opaque", "kept"], modelProfiles: [profile, { model: "kept", capability: "text", protocol: "openai-response" }],
    });
    const config = normalizeConfigSnapshot({ config: {
        ...defaultConfig, channels: [custom, previous], videoModel: "custom::existing", textModel: "aihubkit-video::kept",
    } }).config;
    const imported = importHubModels(config, "video", [
        { id: "opaque", supported_endpoint_types: ["openai"] },
        { id: "added", supported_endpoint_types: ["openai-video"] },
    ], {}, definitions);

    expect(filterModelsByCapability(imported.models, "video", imported.channels)).toEqual([
        "custom::existing", "aihubkit-video::opaque", "aihubkit-video::added",
    ]);
    expect(imported.videoModel).toBe("custom::existing");
    expect(imported.textModel).toBe("aihubkit-video::kept");
    expect(imported.channels[0]).toEqual(custom);
    expect(imported.channels[1]).toMatchObject({ name: "My group", credentialRef: "aihubkit:video", apiKey: "" });
    expect(imported.channels[1]!.modelProfiles).toContainEqual(profile);
    expect(imported.channels[1]!.modelProfiles).toContainEqual(previous.modelProfiles![1]);
    expect(config.channels[1]!.models).toEqual(["opaque", "kept"]);
});

test("the account list reads all saved groups and manual imports after reloading", () => {
    let config = normalizeConfigSnapshot({ config: {
        ...defaultConfig,
        channels: [
            createModelChannel({ id: "aihubkit-default-企业版", credentialRef: "aihubkit:default-企业版", models: ["gpt-5.10"], modelProfiles: [{ model: "gpt-5.10", capability: "text", protocol: "chat-completion" }] }),
            createModelChannel({ id: "aihubkit-video_官key", credentialRef: "aihubkit:video_官key", models: ["sd-mini-z"], modelProfiles: [{ model: "sd-mini-z", capability: "video", protocol: "aihubkit-video" }] }),
            createModelChannel({ id: "custom", models: ["private"] }),
        ],
        textModel: "aihubkit-default-企业版::gpt-5.10", videoModel: "aihubkit-video_官key::sd-mini-z",
    } }).config;
    config = importHubModels(config, "gpt_image_web", [{ id: "gpt-image-2-w" }], { "gpt-image-2-w": "openai-image" }, definitions);
    config = normalizeConfigSnapshot({ config: JSON.parse(JSON.stringify(config)) }).config;
    expect(importedHubModels(config)).toEqual([
        { id: "aihubkit-default-企业版::gpt-5.10", model: "gpt-5.10", group: "default-企业版", protocol: "chat-completion", defaults: ["text"] },
        { id: "aihubkit-video_官key::sd-mini-z", model: "sd-mini-z", group: "video_官key", protocol: "aihubkit-video", defaults: ["video"] },
        { id: "aihubkit-gpt_image_web::gpt-image-2-w", model: "gpt-image-2-w", group: "gpt_image_web", protocol: "openai-image", defaults: ["image"] },
    ]);
    config.videoModel = "";
    expect(importedHubModels(config)[1]!.defaults).toEqual([]);
});
