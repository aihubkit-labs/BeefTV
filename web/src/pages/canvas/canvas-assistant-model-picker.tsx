import { Select } from "antd";
import { assistantModelOptions, resolveAssistantModel } from "@/lib/assistant-model";
import { modelDisplayName, resolveModelChannel, useConfigStore } from "@/stores/use-config-store";

export function CanvasAssistantModelPicker({ busy }: { busy: boolean }) {
    const config = useConfigStore((state) => state.config);
    const selected = resolveAssistantModel(config);
    const options = assistantModelOptions(config).map((value) => ({
        value,
        label: modelDisplayName(config, value),
        channelName: resolveModelChannel(config, value).name,
        title: `${modelDisplayName(config, value)} · ${resolveModelChannel(config, value).name}`,
    }));
    return (
        <Select
            className="canvas-assistant-model"
            aria-label="助手模型"
            size="small"
            variant="borderless"
            value={selected || undefined}
            placeholder={options.length ? "选择助手模型" : "暂无可用模型"}
            options={options}
            optionRender={(option) => (
                <div className="max-w-80 whitespace-normal break-words py-1">
                    <div>{option.data.label}</div>
                    <div className="text-xs text-foreground/60">{option.data.channelName}</div>
                </div>
            )}
            disabled={busy || options.length === 0}
            placement="topLeft"
            popupMatchSelectWidth={false}
            onChange={(value) => {
                if (!busy && assistantModelOptions(useConfigStore.getState().config).includes(value)) {
                    useConfigStore.getState().updateConfig("assistantModel", value);
                }
            }}
        />
    );
}
