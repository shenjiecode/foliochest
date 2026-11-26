import { Download, File, Plus, Trash2 } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, IconButton, SelectField } from "@/components/common";
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
    <div className="property-panel field-grid">
      <Field
        id="entry-key"
        label="Key"
        note="Key、描述和分组为明文，请勿填写秘密。"
      >
        <Input
          id="entry-key"
          value={entry.key}
          required
          readOnly={!isNew}
          onChange={(e) => onChange({ key: e.target.value })}
          spellCheck={false}
        />
      </Field>
      <Field id="entry-description" label="描述">
        <Textarea
          id="entry-description"
          value={entry.description}
          readOnly={!editable}
          required
          rows={3}
          onChange={(e) => onChange({ description: e.target.value })}
        />
      </Field>
      <GroupField
        value={entry.group || ""}
        onChange={(group) => onChange({ group })}
        groups={groups}
        format={format}
        disabled={!editable}
      />
      <Field id="entry-format" label="正文格式">
        <SelectField
          id="entry-format"
          value={entry.format}
          disabled={!editable}
          onChange={(e) =>
            onChange({ format: e.target.value as Entry["format"] })
          }
        >
          <option value="text">纯文本</option>
          <option value="markdown">Markdown</option>
          <option value="json">JSON</option>
        </SelectField>
      </Field>
      <div className="property-section">
        <h3 className="section-heading">AI 权限</h3>
        <div className="field-pair">
          {(["read_policy", "write_policy"] as const).map((field, i) => (
            <Field
              key={field}
              id={field}
              label={i === 0 ? "读取正文与附件" : "修改内容"}
            >
              <SelectField
                id={field}
                value={entry[field]}
                disabled={!editable}
                onChange={(e) =>
                  onChange({ [field]: e.target.value as Policy })
                }
              >
                {Object.entries(policyLabels).map(([v, label]) => (
                  <option key={v} value={v}>
                    {label}
                  </option>
                ))}
              </SelectField>
            </Field>
          ))}
        </div>
      </div>
    </div>
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
