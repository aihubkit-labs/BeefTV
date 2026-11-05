import { afterEach, expect, test } from "bun:test";
import { useAppearanceStore } from "@/stores/use-appearance-store";
import { assistantStatusNotice } from "@/pages/canvas/canvas-assistant-copy";
import { beefAPIConnectionLabel } from "@/services/api/beefapi-connection";
import { agentClientModeSummary, agentClientSetupBlock } from "@/pages/agents/agent-client-presentation";
import { requirementLabel } from "@/pages/skills/market-copy";

const original = useAppearanceStore.getState().appearance;
afterEach(() => useAppearanceStore.getState().setAppearance(original));

test("status and helper copy use the live client name and preserve local defaults", () => {
    for (const name of ["ZModelTV", "shanzhuTV", "云大TV", original.brandName]) {
        useAppearanceStore.getState().setAppearance({ ...original, brandName: name });
        expect(assistantStatusNotice("credential_missing").text).toBe(`${name} 还没连接好`);
        expect(beefAPIConnectionLabel({ state: "connecting" })).toBe(`正在连接 ${name}`);
        expect(agentClientModeSummary("read-write")).toContain(name);
        expect(agentClientSetupBlock({ kind: "codex", title: "连接", command: "beeftv mcp" })?.instruction).toContain(name);
        expect(requirementLabel("beeftv.canvas")).toBe(`${name} 画布工具`);
    }
});
