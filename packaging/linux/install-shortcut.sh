#!/bin/sh
set -eu
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# Desktop Entry has two escaping layers. Refuse unusual paths instead of
# generating an ambiguous command; the application itself still accepts them.
case "$base" in *'"'*|*'\'*|*'`'*|*'$'*|*'%'*) echo '请将解压目录放在不含引号、反斜杠、反引号、美元符或百分号的位置。' >&2; exit 1;; esac
if printf '%s' "$base" | LC_ALL=C grep -q '[[:cntrl:]]'; then echo '路径不能包含控制字符。' >&2; exit 1; fi
data=${XDG_DATA_HOME:-"$HOME/.local/share"}
case "$data" in /*) ;; *) echo 'XDG_DATA_HOME 必须是绝对路径。' >&2; exit 1;; esac
mkdir -p "$data/applications"
file="$data/applications/foliochest.desktop"
if [ -e "$file" ] && ! grep -q '^X-FolioChest-Shortcut=true$' "$file"; then echo '同名应用入口不属于资料匣，未覆盖。' >&2; exit 1; fi
if [ "${1:-}" = '--remove' ]; then
  rm -f -- "$file"
  echo '已移除应用菜单入口，程序和资料未删除。'
  exit 0
fi
chmod u+x "$base/foliochest"
tmp=$(mktemp "$data/applications/.foliochest.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT HUP INT TERM
cat > "$tmp" <<EOF
[Desktop Entry]
Type=Application
Name=资料匣
GenericName=本地加密资料库
Comment=FolioChest
Exec="$base/foliochest"
Icon=$base/foliochest.png
Terminal=false
Categories=Utility;
StartupNotify=false
X-FolioChest-Shortcut=true
EOF
chmod 644 "$tmp"
mv -f -- "$tmp" "$file"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$data/applications" >/dev/null 2>&1 || true
printf '已添加应用菜单入口：资料匣\n请保留当前解压目录，移动后重新运行本脚本。未开启登录自启动。\n'
