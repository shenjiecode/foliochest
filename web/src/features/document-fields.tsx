import { useRef } from "react";
import { Button } from "@/components/ui/button";
import { Download, File, Plus, Trash2, UploadCloud, Info } from "lucide-react";
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
        <div className="property-policies">
          {(["read_policy", "write_policy"] as const).map((field, i) => (
            <div className="property-policy" key={field}>
              <ChoiceField
                id={field}
                label={i === 0 ? "AI读取权限" : "AI修改权限"}
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
          <IconButton
            type="button"
            label="Key、描述和分组以明文保存，请勿填写敏感信息。"
          >
            <Info />
          </IconButton>
        </div>
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
  const input = useRef<HTMLInputElement>(null);
  return (
    <div className="attachments-panel">
      <div className="attachment-heading">
        <h3>附件 · {files.length}</h3>
        {editable && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={busy}
            onClick={() => input.current?.click()}
          >
            <Plus />
            添加附件
          </Button>
        )}
      </div>
      {!files.length && !editable && <p className="field-note">没有附件</p>}
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
        <>
          <label className="attachment-dropzone">
            <UploadCloud />
            <span>
              {busy ? "正在处理附件…" : "点击选择文件，或拖拽文件到编辑区"}
            </span>
            <input
              ref={input}
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
          <p className="attachment-limits">
            单个最大 2 MiB，最多 16 个，保存后生效。
          </p>
        </>
      )}
    </div>
  );
}
