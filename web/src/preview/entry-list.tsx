import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  FileCode2,
  FileText,
  ListFilter,
  Plus,
  Search,
  SearchX,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { EmptyState, IconButton } from "./shared";
import type { Entry } from "./model";
export function EntryList({
  entries,
  group,
  query,
  bodySearch,
  trash,
  selected,
  checked,
  onQuery,
  onBodySearch,
  onSelect,
  onCheck,
  onNew,
  onMove,
  onClear,
}: {
  entries: Entry[];
  group: string | null;
  query: string;
  bodySearch: boolean;
  trash: boolean;
  selected: string | null;
  checked: Set<string>;
  onQuery: (value: string) => void;
  onBodySearch: (value: boolean) => void;
  onSelect: (id: string) => void;
  onCheck: (id: string, value: boolean) => void;
  onNew: () => void;
  onMove: () => void;
  onClear: () => void;
}) {
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const groups =
    group === null
      ? [...new Set(entries.map((entry) => entry.group))]
      : [group];
  return (
    <section
      className={cn("entry-list", checked.size > 0 && "selection-mode")}
      aria-label={trash ? "已删除资料" : "资料列表"}
    >
      <header className="list-header">
        <div className="list-title">
          <h1>
            {trash ? "回收站" : group === null ? "全部资料" : group || "未分组"}
          </h1>
          {!trash && (
            <IconButton label="新建条目" onClick={onNew}>
              <Plus />
            </IconButton>
          )}
        </div>
        <div className="search-box">
          <Search />
          <Input
            id="entry-search"
            type="search"
            aria-label="搜索资料"
            placeholder={bodySearch ? "搜索资料与正文…" : "搜索资料…"}
            value={query}
            onChange={(event) => onQuery(event.target.value)}
            autoComplete="off"
          />
          {!trash && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="search-options"
                  aria-label="搜索范围"
                >
                  <ListFilter />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>搜索范围</DropdownMenuLabel>
                <DropdownMenuCheckboxItem
                  checked={bodySearch}
                  onCheckedChange={onBodySearch}
                >
                  包含正文
                </DropdownMenuCheckboxItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
      </header>
      <div className="list-results scroll-region">
        {entries.length === 0 ? (
          <EmptyState
            icon={<SearchX />}
            title={query ? "没有匹配的资料" : trash ? "回收站为空" : "暂无资料"}
            action={
              query ? (
                <Button variant="outline" size="sm" onClick={() => onQuery("")}>
                  清除搜索
                </Button>
              ) : undefined
            }
          />
        ) : (
          groups.map((name) => (
            <section key={name}>
              {group === null && (
                <button
                  className="list-group-heading"
                  aria-expanded={!collapsed.has(name)}
                  onClick={() =>
                    setCollapsed((before) => {
                      const next = new Set(before);
                      if (next.has(name)) next.delete(name);
                      else next.add(name);
                      return next;
                    })
                  }
                >
                  {collapsed.has(name) ? <ChevronRight /> : <ChevronDown />}
                  <span>{name || "未分组"}</span>
                  <span>
                    · {entries.filter((entry) => entry.group === name).length}
                  </span>
                </button>
              )}
              {(!collapsed.has(name) || group !== null) &&
                entries
                  .filter((entry) => entry.group === name)
                  .map((entry) => (
                    <div
                      key={entry.id}
                      className={cn(
                        "entry-row",
                        selected === entry.id && "selected",
                      )}
                    >
                      <button
                        className="entry-open"
                        onClick={() => onSelect(entry.id)}
                        aria-current={
                          selected === entry.id ? "true" : undefined
                        }
                      >
                        <span className="entry-key">
                          {entry.type === "JSON" ? <FileCode2 /> : <FileText />}
                          <span>{entry.key}</span>
                        </span>
                        <span className="entry-description">
                          {entry.description || "无描述"}
                        </span>
                      </button>
                      {!trash && (
                        <Checkbox
                          className="entry-checkbox"
                          aria-label={"选择 " + entry.key}
                          checked={checked.has(entry.id)}
                          onCheckedChange={(value) =>
                            onCheck(entry.id, value === true)
                          }
                        />
                      )}
                    </div>
                  ))}
            </section>
          ))
        )}
      </div>
      <footer className="list-footer">
        {checked.size ? (
          <>
            <span>已选 {checked.size} 条</span>
            <Button variant="ghost" onClick={onClear}>
              取消
            </Button>
            <Button variant="secondary" onClick={onMove}>
              移动到…
            </Button>
          </>
        ) : (
          <span>
            {entries.length} 条资料{bodySearch && !trash ? " · 包含正文" : ""}
          </span>
        )}
      </footer>
    </section>
  );
}
