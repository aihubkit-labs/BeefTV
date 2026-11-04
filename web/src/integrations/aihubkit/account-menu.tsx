import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Alert, App, Button, Checkbox, Input, Space, Table, Tabs } from "antd";
import { UserRound } from "lucide-react";
import { AppModal } from "@/components/ui/product/app-modal";
import { appQueryClient } from "@/lib/query-client";
import { withGenerationConsumersPaused } from "@/services/generation-consumer-lifecycle";
import { flushModelConfig, getModelConfigPersistenceState } from "@/services/model-config-repository";
import { flushCanvasStorePersistence } from "@/stores/canvas/use-canvas-store";
import { flushAssetStorePersistence } from "@/stores/use-asset-store";
import { hub, setHubSession } from "./api";
import { HubAccountModels } from "./account-models";
import { useHubSession, useUpdateHubSession } from "./gate";
import { useHubBrand } from "./brand";

export function HubAccountMenu({ variant = "default", collapsed = false }: { variant?: "default" | "sidebar"; collapsed?: boolean }) {
    const { platformName } = useHubBrand();
    const session = useHubSession();
    const [open, setOpen] = useState(false);
    if (!session.enabled) return null;
    return <>
        {variant === "sidebar" ? <button type="button" className={`app-workspace-nav-link group relative isolate flex min-h-11 w-full items-center gap-2 rounded-[var(--r-md)] px-3 py-2 text-[var(--fs-body)] text-foreground/62 transition-[color,transform] duration-200 hover:bg-surface-hover hover:text-foreground${collapsed ? " is-collapsed justify-center" : ""}`} aria-label={`${platformName} 账号与额度`} title={`${platformName} 账号与额度`} onClick={() => setOpen(true)}>
            <UserRound className="size-[18px] shrink-0" strokeWidth={1.8} />
            {!collapsed ? <span className="app-workspace-nav-title truncate">{platformName} 账号</span> : null}
        </button> : <Button block type="text" onClick={() => setOpen(true)}>{platformName} 账号与额度</Button>}
        <AppModal title={`${platformName} 账号`} open={open} onCancel={() => setOpen(false)} footer={null} width={760}>
            {open && <AccountPanel />}
        </AppModal>
    </>;
}

function AccountPanel() {
    const { platformName, appName } = useHubBrand();
    const session = useHubSession();
    const updateSession = useUpdateHubSession();
    const { message } = App.useApp();
    const [page, setPage] = useState(1);
    const [tab, setTab] = useState("account");
    const [redemption, setRedemption] = useState("");
    const [busy, setBusy] = useState(false);
    const [leaving, setLeaving] = useState(false);
    const [error, setError] = useState("");
    const self = useQuery({ queryKey: ["aihubkit", session.scope, "self"], queryFn: hub.self });
    const logs = useQuery({ queryKey: ["aihubkit", session.scope, "logs", page], queryFn: () => hub.logs(page), enabled: tab === "logs" });
    const action = async (operation: () => Promise<void>) => {
        setBusy(true); setError("");
        try { await operation(); }
        catch (cause) { setError(cause instanceof Error ? cause.message : "操作失败，请重试"); }
        finally { setBusy(false); }
    };
    const logout = () => action(async () => {
        setLeaving(true);
        try {
            await withGenerationConsumersPaused(async () => {
                await Promise.all([flushCanvasStorePersistence(), flushAssetStorePersistence(), flushModelConfig()]);
                if (getModelConfigPersistenceState().dirty) throw new Error("模型配置尚未保存，请重试保存后退出");
                await appQueryClient.cancelQueries();
                await hub.logout();
                setHubSession();
                appQueryClient.clear();
                // Reload destroys all old writers before another account can log in.
                window.location.reload();
            });
        } catch (cause) { setLeaving(false); throw cause; }
    });
    return <div className="space-y-4" aria-busy={busy}>
        {(error || self.error || logs.error) && <Alert type="error" showIcon title={error || self.error?.message || logs.error?.message} />}
        <Tabs activeKey={tab} onChange={setTab} items={[
            { key: "account", label: "账号与充值", children: <div className="space-y-4">
                <p>{session.user?.display_name || session.user?.username} · @{session.user?.username}</p>
                <p>可用额度：{self.data ? (self.data.quota / 500000).toFixed(4) : "加载中"}　已用额度：{self.data ? (self.data.used_quota / 500000).toFixed(4) : "—"}</p>
                <Button loading={self.isFetching} disabled={busy} onClick={() => void self.refetch()}>刷新额度</Button>
                <Space.Compact block><Input.Password aria-label="兑换码" placeholder="输入兑换码" value={redemption} onChange={(event) => setRedemption(event.target.value)} disabled={busy} autoComplete="off" />
                    <Button type="primary" disabled={!redemption.trim() || busy} loading={busy} onClick={() => void action(async () => { await hub.topup(redemption.trim()); setRedemption(""); await self.refetch(); void message.success("充值成功"); })}>兑换充值</Button></Space.Compact>
                <section aria-label="素材上传" className="space-y-2 rounded-[var(--r-md)] border border-border p-4">
                    <Checkbox checked={session.uploadToAIHubkit === true} disabled={busy} onChange={(event) => {
                        const checked = event.target.checked;
                        void action(async () => {
                            const next = await hub.setUploadPreference(checked);
                            updateSession(next);
                            void message.success(next.uploadToAIHubkit ? `后续参考素材使用 ${platformName} 存储` : `后续参考素材使用 ${appName} 存储设置`);
                        });
                    }}>上传到 {platformName} 素材库</Checkbox>
                    <p className="text-sm text-foreground/60">生成需要上传参考素材时，勾选使用 {platformName} 文件存储；取消勾选后按 {appName}「存储设置」处理，可使用自己的 S3 桶。</p>
                    <p className="text-sm text-foreground/60">首次登录默认勾选，后续登录保留该账号上次的选择。切换影响后续上传，不搬迁已有素材，也不影响模型导入和调用。</p>
                </section>
                <p className="text-sm text-foreground/60">退出会停止本机任务处理并关闭当前账号工作区。已提交到上游的任务仍可能计费；重新登录同一账号后可继续查看和取回结果。</p>
                <Button danger loading={leaving} disabled={busy} onClick={() => void logout()}>退出登录</Button>
            </div> },
            { key: "models", label: "分组与模型", children: <HubAccountModels /> },
            { key: "logs", label: "使用日志", children: <Table size="small" rowKey="id" loading={logs.isFetching} dataSource={logs.data?.items || []} scroll={{ x: 600 }} pagination={{ current: page, pageSize: 10, total: logs.data?.total || 0, showSizeChanger: false, onChange: setPage }} columns={[
                { title: "时间", dataIndex: "created_at", render: (value: number) => new Date(value * 1000).toLocaleString() },
                { title: "模型", dataIndex: "model_name" }, { title: "分组", dataIndex: "group" },
                { title: "费用（额度）", dataIndex: "quota", render: (value: number) => (value / 500000).toFixed(6) },
            ]} /> },
        ]} />
        {leaving && <AppModal open title="正在退出" footer={null} closable={false} maskClosable={false} keyboard={false}><p>正在保存并关闭当前工作区，请等待应用重新加载。</p></AppModal>}
    </div>;
}
