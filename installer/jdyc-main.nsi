; 揭东一初联机大厅 —— 主流版安装包（Flutter Windows）
; 输入：仓库根 dist-flutter\（Flutter job 产物 + 依赖）
; 输出：installer\JDYC-Hub-Win10-v1.0.0.exe
Unicode true
ManifestDPIAware true

!define APPNAME "揭东一初联机大厅"
!define VERSION "1.0.0"

Name "${APPNAME}"
OutFile "installer\JDYC-Hub-Win10-v1.0.0.exe"
InstallDir "$PROGRAMFILES64\JDYCHub"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

Page directory
Page instfiles

Section "安装"
  SetOutPath "$INSTDIR"
  ; 完整拷贝 dist-flutter（含 jdyc_hub.exe、flutter_windows.dll、
  ; data\、jdyc-core.dll、frpc.exe、edge.exe、TAP 安装包）
  File /r "dist-flutter\*.*"

  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\jdyc_hub.exe"
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\jdyc_hub.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\JDYCHub" \
    "DisplayName" "${APPNAME}"
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
