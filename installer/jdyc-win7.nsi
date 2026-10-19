; 揭东一初联机大厅 —— Win7 专用版安装包（Qt 5.15）
; 输入：仓库根 dist-qt\（Qt job 产物 + 依赖，见 ci-clients.yml package job）
; 输出：installer\JDYC-Hub-Win7-v1.0.0.exe
Unicode true
ManifestDPIAware true

!define APPNAME "揭东一初联机大厅"
!define VERSION "1.0.0"

Name "${APPNAME}（Win7 专用版）"
OutFile "installer\JDYC-Hub-Win7-v1.0.0.exe"
InstallDir "$PROGRAMFILES64\JDYCHub"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

Page directory
Page instfiles

Section "安装"
  SetOutPath "$INSTDIR"
  ; 完整拷贝 dist-qt（含 JDYCHub.exe、Qt DLL、jdyc-core.dll、frpc.exe、edge.exe、TAP 安装包）
  File /r "dist-qt\*.*"

  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\JDYCHub.exe"
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\JDYCHub.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; 写注册表卸载信息
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\JDYCHub" \
    "DisplayName" "${APPNAME}（Win7 专用版）"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\JDYCHub" \
    "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\JDYCHub" \
    "DisplayVersion" "${VERSION}"
SectionEnd

Section "Uninstall"
  RMDir /r "$INSTDIR"
  RMDir /r "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\JDYCHub"
SectionEnd
