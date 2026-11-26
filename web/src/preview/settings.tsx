import { useState, type ReactNode } from "react";
import { ArchiveRestore, Download, KeyRound, Plug, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { SelectField } from "./shared";
import type { DialogKind } from "./model";

function SettingRow({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <div className="setting-row">
      <div>
        <h3>{title}</h3>
        {description && <p>{description}</p>}
      </div>
      {children}
    </div>
  );
}
export function SettingsView({
  onOpen,
  notice,
}: {
  onOpen: (kind: DialogKind) => void;
  notice: (message: string) => void;
}) {
  const [system, setSystem] = useState(true);
  const [startup, setStartup] = useState(false);
  const [autoUnlock, setAutoUnlock] = useState(false);
  const [remember, setRemember] = useState(false);
  const [idle, setIdle] = useState("0");
  const change = (set: (value: boolean) => void, value: boolean) => {
    set(value);
    notice("仅更新预览开关，未修改本机设置。");
  };
  return (
    <section className="settings-shell">
      <header className="page-heading">
        <h1>设置</h1>
      </header>
      <Tabs defaultValue="vault" className="settings-tabs">
        <TabsList aria-label="设置分类">
          <TabsTrigger value="vault">资料库</TabsTrigger>
          <TabsTrigger value="security">安全</TabsTrigger>
          <TabsTrigger value="device">设备</TabsTrigger>
          <TabsTrigger value="mcp">MCP</TabsTrigger>
        </TabsList>
        <TabsContent value="vault" className="settings-body">
          <section className="settings-section">
            <h2>当前资料库</h2>
            <SettingRow title="个人资料" description="/示例资料库/个人资料">
              <Button variant="outline" onClick={() => onOpen("open-vault")}>
                切换资料库
              </Button>
            </SettingRow>
            <SettingRow title="资料库格式" description="格式 3 · 示例">
              <span className="muted-text text-xs">无需升级</span>
            </SettingRow>
          </section>
          <section className="settings-section">
            <h2>备份与恢复</h2>
            <SettingRow
              title="下载备份"
              description="包含加密内容、附件和历史；元信息保持明文。"
            >
              <Button variant="outline" disabled>
                <Download />
                下载备份
              </Button>
            </SettingRow>
            <SettingRow
              title="从备份恢复"
              description="恢复到新的空目录，不覆盖当前资料。"
            >
              <Button variant="outline" onClick={() => onOpen("restore-vault")}>
                <ArchiveRestore />
                恢复
              </Button>
            </SettingRow>
          </section>
        </TabsContent>
        <TabsContent value="security" className="settings-body">
          <section className="settings-section">
            <h2>锁定</h2>
            <SettingRow
              title="跟随系统锁定"
              description="系统锁屏、会话切换或休眠时锁定。"
            >
              <Switch
                aria-label="跟随系统锁定"
                checked={system}
                onCheckedChange={(value) => change(setSystem, value)}
              />
            </SettingRow>
            <SettingRow title="网页空闲锁定">
              <SelectField
                aria-label="网页空闲锁定"
                value={idle}
                onChange={(event) => {
                  setIdle(event.target.value);
                  notice("仅更新预览选项。");
                }}
              >
                <option value="0">关闭</option>
                <option value="5">5 分钟</option>
                <option value="15">15 分钟</option>
                <option value="30">30 分钟</option>
              </SelectField>
            </SettingRow>
          </section>
          <section className="settings-section">
            <h2>主密码</h2>
            <SettingRow title="修改主密码">
              <Button variant="outline" onClick={() => onOpen("password")}>
                <KeyRound />
                修改
              </Button>
            </SettingRow>
          </section>
        </TabsContent>
        <TabsContent value="device" className="settings-body">
          <section className="settings-section">
            <h2>这台设备</h2>
            <SettingRow
              title="登录系统后启动"
              description="仅启动后台，不自动打开网页。"
            >
              <Switch
                aria-label="登录系统后启动"
                checked={startup}
                onCheckedChange={(value) => change(setStartup, value)}
              />
            </SettingRow>
            <SettingRow
              title="服务启动时自动解锁"
              description="正式启用时需验证主密码，并使用系统凭据库。"
            >
              <Switch
                aria-label="服务启动时自动解锁"
                checked={autoUnlock}
                onCheckedChange={(value) => change(setAutoUnlock, value)}
              />
            </SettingRow>
            <SettingRow
              title="记住这个浏览器"
              description="90 天内可在后台自动解锁后进入网页。"
            >
              <Switch
                aria-label="记住这个浏览器"
                checked={remember}
                onCheckedChange={(value) => change(setRemember, value)}
              />
            </SettingRow>
          </section>
          <p className="settings-footnote">
            当前为开关样式预览，不登记启动项、不创建设备凭据。
          </p>
        </TabsContent>
        <TabsContent value="mcp" className="settings-body">
          <section className="settings-section">
            <h2>客户端接入</h2>
            <SettingRow
              title="MCP 地址"
              description="http://127.0.0.1:7463/mcp"
            >
              <Plug size={18} className="muted-text" />
            </SettingRow>
            <SettingRow
              title="接入凭据"
              description="创建和撤销操作将在功能迁移时接入。"
            >
              <Button variant="outline" disabled>
                <Plus />
                接入客户端
              </Button>
            </SettingRow>
          </section>
        </TabsContent>
      </Tabs>
    </section>
  );
}
