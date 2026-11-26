import { Download, File, Plus, Trash2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, IconButton } from "@/components/common";
import { ChoiceField } from "@/components/choice-field";
import { GroupField } from "@/components/forms";
import {
  type Entry,
  type Group,
  type Policy,
  policyLabels,
  type Attachment,
} from "@/lib/types";
import { downloadAttachment, fileSize } from "@/lib/files";
export function Properties({
  entry,
  groups,
  format,
  editable,
  isNew,
  onChange,
}: {
  entry: Entry;
  groups: Group[];
  format: number;
  editable: boolean;
  isNew: boolean;
  onChange: (patch: Partial<Entry>) => void;
}) {
  return (
    <section className="property-card" aria-label="资料属性">
      <header className="property-card-heading">
        <h3>属性</h3>
        <p>Key、描述和分组为明文，请勿填写秘密。</p>
      </header>
      <div className="property-fields">
        <div className="property-key">
          <Field id="entry-key" label="Key">
            <Input
              id="entry-key"
              value={entry.key}
              required
              readOnly={!isNew}
              onChange={(e) => onChange({ key: e.target.value })}
              spellCheck={false}
            />
          </Field>
        </div>
        <div className="property-group">
          <GroupField
            value={entry.group || ""}
            onChange={(group) => onChange({ group })}
            groups={groups}
            format={format}
            disabled={!editable}
            compact
          />
        </div>
        <div className="property-description">
          <Field id="entry-description" label="描述">
            <Textarea
              id="entry-description"
              value={entry.description}
              readOnly={!editable}
              required
              rows={1}
              className="property-description-input"
              onChange={(e) => onChange({ description: e.target.value })}
            />
          </Field>
        </div>
        {(["read_policy", "write_policy"] as const).map((field, i) => (
          <div className="property-policy" key={field}>
            <ChoiceField
              id={field}
              label={i === 0 ? "AI 读取正文与附件" : "AI 修改内容"}
              value={entry[field]}
              disabled={!editable}
              options={Object.entries(policyLabels).map(([value, label]) => ({
                value: value as Policy,
                label,
              }))}
              onChange={(value) => onChange({ [field]: value })}
            />
          </div>
        ))}
      </div>
    </section>
  );
}
export function Attachments({
  files,
  editable,
  busy,
  onAdd,
  onRemove,
}: {
  files: Attachment[];
  editable?: boolean;
  busy?: boolean;
  onAdd?: (files: File[]) => void;
  onRemove?: (index: number) => void;
}) {
  return (
    <div className="attachments-panel">
      <div className="attachment-heading">
        <h3>附件 · {files.length}</h3>
        {editable && (
          <label className="upload-control">
            <Plus />
            <span>添加附件</span>
            <input
              aria-label="添加附件"
              type="file"
              multiple
              disabled={busy}
              onChange={(e) => {
                const items = [...(e.target.files || [])];
                e.target.value = "";
                onAdd?.(items);
              }}
            />
          </label>
        )}
      </div>
      {!files.length && <p className="field-note">没有附件</p>}
      {files.map((file, i) => (
        <div className="attachment-row" key={file.name}>
          <File />
          <div>
            <p>{file.name}</p>
            <small>{Math.ceil(fileSize(file.data) / 1024)} KiB</small>
          </div>
          <IconButton
            label={"下载 " + file.name}
            onClick={() => downloadAttachment(file)}
            type="button"
          >
            <Download />
          </IconButton>
          {editable && (
            <IconButton
              label={"移除 " + file.name}
              disabled={busy}
              onClick={() => onRemove?.(i)}
              type="button"
            >
              <Trash2 />
            </IconButton>
          )}
        </div>
      ))}
      {editable && (
        <p className="field-note">
          每个最多 2 MiB，每条最多 16 个。下载内容为明文，请保存在可信位置。
        </p>
      )}
    </div>
  );
}
