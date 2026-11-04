import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Alert, App, Button, Select, Space, Table, Tag } from "antd";
import { modelProtocolLabel, protocolGroups } from "@/lib/model-protocols";
import { fetchPluginProviderCatalog } from "@/services/api/plugin-catalog";
import { commitModelConfig, flushModelConfig, getModelConfigPersistenceState } from "@/services/model-config-repository";
import { useConfigStore } from "@/stores/use-config-store";
import { hub, type HubModel } from "./api";
import { hubModelProtocol, importedHubModels, importHubModels, resetHubModels } from "./catalog";
import { useHubBrand } from "./brand";
import { useHubSession } from "./gate";

export function HubAccountModels() {
    const brand = useHubBrand();
    const session = useHubSession();
    const config = useConfigStore((state) => state.config);
    const { message } = App.useApp();
    const [group, setGroup] = useState(session.user?.group || "");
    const [models, setModels] = useState<HubModel[]>([]);
    const [selected, setSelected] = useState<React.Key[]>([]);
    const [overrides, setOverrides] = useState<Record<string, string>>({});
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [warnings, setWarnings] = useState<string[]>([]);
    const protocols = useQuery({ queryKey: ["aihubkit", session.scope, "protocols"], queryFn: () => fetchPluginProviderCatalog("user.custom-channel") });
    const imported = importedHubModels(config);
    const action = async (operation: () => Promise<void>) => {
        setBusy(true); setError("");
        try { await operation(); }
        catch (cause) { setError(cause instanceof Error ? cause.message : "操作失败，请重试"); }
        finally { setBusy(false); }
    };
    return <div className="space-y-4" aria-busy={busy}>
        {(error || protocols.error) && <Alert type="error" showIcon title={error || protocols.error?.message} />}
        {!!warnings.length && <Alert type="warning" showIcon title="部分默认模型未导入" description={warnings.map((warning) => <p key={warning}>{warning}</p>)} />}
        <section className="space-y-3" aria-label="已导入模型">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <h3 className="font-medium">已导入模型</h3>
                <Button aria-label="初始化默认模型配置" loading={busy} disabled={busy} onClick={() => void action(async () => {
                    await flushModelConfig();
                    if (getModelConfigPersistenceState().dirty) throw new Error(getModelConfigPersistenceState().error || "当前模型配置未保存，请重试");
                    const result = await hub.defaultModelConfig();
                    const next = resetHubModels(useConfigStore.getState().config, result.config);
                    await commitModelConfig(next);
                    if (getModelConfigPersistenceState().dirty) throw new Error(getModelConfigPersistenceState().error || "初始化配置未保存，请重试");
                    useConfigStore.getState().replaceConfig(next);
                    setModels([]); setSelected([]); setOverrides({}); setWarnings(result.warnings);
                    if (!result.warnings.length) void message.success("文本、图片、视频默认模型已初始化");
                })}>初始化默认模型配置</Button>
            </div>
            <p className="text-sm text-foreground/60">初始化会清除通过 {brand.platformName} 账号导入的全部模型，并重新配置默认文本、图片和视频模型。独立配置的其他渠道保留。</p>
            <Table size="small" rowKey="id" dataSource={imported} pagination={{ pageSize: 10 }} scroll={{ x: 600 }} columns={[
                { title: "模型", dataIndex: "model" },
                { title: "分组", dataIndex: "group", render: (value: string) => <span title={session.groups?.[value]?.desc}>{value}</span> },
                { title: "协议", dataIndex: "protocol", render: (value: string | undefined) => modelProtocolLabel(value, protocols.data) },
                { title: "默认用途", render: (_, row) => row.defaults.length ? row.defaults.map((capability) => <Tag key={capability}>{({ text: "文本", image: "图片", video: "视频", audio: "音频" })[capability]}</Tag>) : "—" },
            ]} />
        </section>
        <section className="space-y-3" aria-label="添加分组模型">
            <h3 className="font-medium">添加分组模型</h3>
            <p className="text-sm text-foreground/60">选择账号允许的分组并获取模型，再选择协议导入。已导入的模型和默认用途显示在上方，可在模型配置中调整。</p>
            <Space wrap><Select
                aria-label="模型分组"
                value={group}
                className="min-w-48"
                disabled={busy}
                placeholder="输入分组名称搜索"
                showSearch={{ optionFilterProp: ["label", "description"] }}
                onChange={(value) => { setGroup(value); setModels([]); setSelected([]); setOverrides({}); }}
                options={Object.entries(session.groups || {}).map(([value, entry]) => ({ value, label: `${value} · 倍率 ${entry.ratio}`, description: entry.desc, title: entry.desc }))}
            />
                <Button loading={busy} disabled={busy || !group} onClick={() => void action(async () => {
                    const catalog = await hub.catalog(group);
                    const saved = Object.fromEntries(imported.filter((row) => row.group === group && row.protocol).map((row) => [row.model, row.protocol!]));
                    setModels(catalog); setOverrides(saved);
                    setSelected(catalog.filter((model) => saved[model.id] || hubModelProtocol(model)).map((model) => model.id));
                })}>获取模型</Button>
                <Button type="primary" disabled={busy || selected.length === 0 || !protocols.data} onClick={() => void action(async () => {
                    const next = importHubModels(useConfigStore.getState().config, group, models.filter((model) => selected.includes(model.id)), overrides, protocols.data || [], brand);
                    useConfigStore.getState().replaceConfig(next);
                    await commitModelConfig(next);
                    if (getModelConfigPersistenceState().dirty) throw new Error(getModelConfigPersistenceState().error || "模型配置未保存，请重试");
                    void message.success("模型已导入，可在模型设置中选用");
                })}>导入所选模型</Button>
            </Space>
            {!!models.length && <Table size="small" rowKey="id" dataSource={models} pagination={{ pageSize: 10 }} scroll={{ x: 600 }} rowSelection={{ selectedRowKeys: selected, onChange: setSelected }} columns={[
                { title: "模型", dataIndex: "id" },
                { title: "协议", render: (_, model: HubModel) => <Select
                    aria-label={`${model.id} 的协议`}
                    className="min-w-56"
                    disabled={busy}
                    placeholder="输入协议名称或请求路径搜索"
                    showSearch={{ optionFilterProp: "label" }}
                    value={overrides[model.id] || hubModelProtocol(model)}
                    options={protocolGroups(protocols.data || [])}
                    onChange={(value) => setOverrides((current) => ({ ...current, [model.id]: value }))}
                /> },
            ]} />}
        </section>
    </div>;
}
