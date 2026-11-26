Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
!include "WinVer.nsh"

Name "资料匣 · FolioChest"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\FolioChest"
RequestExecutionLevel user
SetCompressor /SOLID zlib
AllowSkipFiles off
Icon "${ICON}"
UninstallIcon "${ICON}"
!define APPKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\FolioChest"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TEXT "安装资料匣到当前用户。安装不会启动后台或开启登录自启动。升级前请保存资料并在网页退出程序。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${STAGE}/LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!define MUI_UNCONFIRMPAGE_TEXT_TOP "请先退出资料匣。卸载保留资料库和本机设备配置；如需移除设备凭据，请先在网页关闭设备授权。"
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  ${IfNot} ${AtLeastWin10}
    MessageBox MB_OK|MB_ICONSTOP "需要 Windows 10 或更新系统。"
    Abort
  ${EndIf}
  !if "${ARCH}" == "amd64"
    ${IfNot} ${RunningX64}
      MessageBox MB_OK|MB_ICONSTOP "这是 64 位安装包，请使用 x86 安装包。"
      Abort
    ${EndIf}
  !endif
  ${If} ${RunningX64}
    SetRegView 64
  ${EndIf}
  SetShellVarContext current
  ReadRegStr $0 HKCU "${APPKEY}" "Architecture"
  ${If} $0 != ""
  ${AndIf} $0 != "${ARCH}"
    MessageBox MB_OK|MB_ICONSTOP "切换 x86/x64 前请先卸载已安装版本。资料库不会被删除。"
    Abort
  ${EndIf}
FunctionEnd

!macro CheckStopped
  IfFileExists "$INSTDIR\foliochest.exe" 0 done
  ClearErrors
  FileOpen $0 "$INSTDIR\foliochest.exe" a
  IfErrors blocked
  FileClose $0
  Goto done
blocked:
  MessageBox MB_OK|MB_ICONSTOP "程序仍在运行或文件不可写。请在网页退出资料匣后重试。不会强制终止进程。"
  Abort
done:
!macroend

Section "资料匣（必选）" Main
  SectionIn RO
  !insertmacro CheckStopped
  SetOutPath "$INSTDIR"
  File /r "${STAGE}/*"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\资料匣"
  CreateShortcut "$SMPROGRAMS\资料匣\资料匣.lnk" "$INSTDIR\foliochest.exe" "" "$INSTDIR\foliochest.exe"
  WriteRegStr HKCU "${APPKEY}" "DisplayName" "资料匣 · FolioChest"
  WriteRegStr HKCU "${APPKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${APPKEY}" "Publisher" "shenjiecode"
  WriteRegStr HKCU "${APPKEY}" "Architecture" "${ARCH}"
  WriteRegStr HKCU "${APPKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${APPKEY}" "DisplayIcon" "$INSTDIR\foliochest.exe"
  WriteRegStr HKCU "${APPKEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "${APPKEY}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${APPKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${APPKEY}" "NoRepair" 1
SectionEnd

Section /o "桌面快捷方式"
  CreateShortcut "$DESKTOP\资料匣.lnk" "$INSTDIR\foliochest.exe" "" "$INSTDIR\foliochest.exe"
  WriteRegDWORD HKCU "${APPKEY}" "DesktopShortcut" 1
SectionEnd

Function un.onInit
  ${If} ${RunningX64}
    SetRegView 64
  ${EndIf}
  SetShellVarContext current
FunctionEnd

Function un.CleanupStartup
  ; Remove only startup values belonging to this executable path.
  StrCpy $0 0
startup_loop:
  EnumRegValue $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" $0
  StrCmp $1 "" startup_done
  StrCpy $2 $1 9
  StrCmp $2 "Keyfolio-" 0 startup_next
  ReadRegStr $2 HKCU "Software\Microsoft\Windows\CurrentVersion\Run" $1
  StrCpy $3 '$\"$INSTDIR\foliochest.exe$\" '
  StrLen $4 $3
  StrCpy $5 $2 $4
  StrCmp $5 $3 startup_remove
  StrCpy $3 "$INSTDIR\foliochest.exe "
  StrLen $4 $3
  StrCpy $5 $2 $4
  StrCmp $5 $3 startup_remove startup_next
startup_remove:
  ClearErrors
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" $1
  IfErrors startup_next
  Goto startup_loop
startup_next:
  IntOp $0 $0 + 1
  Goto startup_loop
startup_done:
FunctionEnd

Section "Uninstall"
  !insertmacro CheckStopped
  ClearErrors
  Delete "$INSTDIR\foliochest.exe"
  IfErrors 0 +3
    MessageBox MB_OK|MB_ICONSTOP "程序文件无法删除，请退出资料匣后重试。"
    Abort
  ${If} ${RunningX64}
    SetRegView 64
    Call un.CleanupStartup
  ${EndIf}
  SetRegView 32
  Call un.CleanupStartup
  ${If} ${RunningX64}
    SetRegView 64
  ${EndIf}
  ; This generated list deletes only files owned by the package.
  ; RMDir is deliberately non-recursive: unknown files and user data survive.
  !include "${UNINSTALL_MANIFEST}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\资料匣\资料匣.lnk"
  RMDir "$SMPROGRAMS\资料匣"
  ReadRegDWORD $0 HKCU "${APPKEY}" "DesktopShortcut"
  ${If} $0 == 1
    Delete "$DESKTOP\资料匣.lnk"
  ${EndIf}
  DeleteRegKey HKCU "${APPKEY}"
SectionEnd
