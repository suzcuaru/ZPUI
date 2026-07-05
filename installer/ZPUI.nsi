; ==============================================================================
;  ZPUI Installer (NSIS)
;  Per-user, no admin required (enables in-app self-update)
;  Bilingual: auto-detects Windows language (Russian / English)
;
;  Features:
;    - MIT License agreement page
;    - Smart version detection: upgrade / reinstall / downgrade
;    - Bilingual UI (auto-detect from system locale)
;    - File-lock check: detects running ZPUI, asks to close
;    - Preserves user data on update
;    - Start Menu + Desktop shortcuts
;
;  Build:
;    makensis /DVERSION=1.0.49 /DDIST=build\dist /DLICENSE=LICENSE
;              /DICON=build\windows\icon.ico /DOUTDIR=build installer\ZPUI.nsi
; ==============================================================================

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WordFunc.nsh"
!include "FileFunc.nsh"

; --- Configurable defines ---
!ifndef VERSION
  !define VERSION "1.0.0"
!endif
!ifndef ARCH
  !define ARCH "win64"
!endif
!ifndef DIST
  !define DIST "build\dist-${ARCH}"
!endif
!ifndef ICON
  !define ICON "build\windows\icon.ico"
!endif
!ifndef OUTDIR
  !define OUTDIR "..\build"
!endif
!ifndef LICENSE
  !define LICENSE "LICENSE"
!endif

; --- Runtime variables ---
Var ExistingVersion
Var ExistingDir
Var UpgradeMode          ; 0=fresh, 1=upgrade, 2=same, 3=downgrade

; ==============================================================================
;  General settings
; ==============================================================================
Name "ZPUI ${VERSION}"
OutFile "${OUTDIR}\ZPUI-Setup-${VERSION}-${ARCH}.exe"
Unicode True
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\ZPUI"
InstallDirRegKey HKCU "Software\ZPUI" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma

BrandingText "ZPUI ${VERSION}  ·  github.com/suzcuaru/ZPUI"

; --- Version info ---
!ifndef VERSION_NUM
  !define VERSION_NUM "${VERSION}"
!endif
VIProductVersion "${VERSION_NUM}.0"
VIAddVersionKey "ProductName" "ZPUI"
VIAddVersionKey "FileDescription" "ZPUI — Zapret DPI bypass controller"
VIAddVersionKey "CompanyName" "SuzucaRU"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 SuzucaRU — MIT License"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

; ==============================================================================
;  Modern UI
; ==============================================================================
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING

; --- Finish page (paths only — text auto-translated by MUI) ---
!define MUI_FINISHPAGE_RUN "$INSTDIR\zpui.exe"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!define MUI_FINISHPAGE_LINK "github.com/suzcuaru/ZPUI"
!define MUI_FINISHPAGE_LINK_LOCATION "https://github.com/suzcuaru/ZPUI"

; ==============================================================================
;  Pages — install
; ==============================================================================
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE}"
!insertmacro MUI_PAGE_COMPONENTS
!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipDirectoryPage
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; --- Pages — uninstall ---
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ==============================================================================
;  Languages (first = default fallback; auto-detected from system locale)
; ==============================================================================
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Russian"

; ==============================================================================
;  LangStrings — custom text (resolved at runtime by $LANGUAGE)
; ==============================================================================
LangString MsgAppRunning ${LANG_ENGLISH} "ZPUI is still running. Please close ZPUI and click Retry to continue."
LangString MsgAppRunning ${LANG_RUSSIAN} "ZPUI запущен. Пожалуйста, закройте ZPUI и нажмите «Повтор» для продолжения."

LangString MsgUpgrade ${LANG_ENGLISH} "An older version of ZPUI ($ExistingVersion) is installed.$\r$\n$\r$\nThis will upgrade to version ${VERSION}.$\r$\nYour settings and data will be preserved.$\r$\n$\r$\nClick OK to continue or Cancel to abort."
LangString MsgUpgrade ${LANG_RUSSIAN} "Установлена более старая версия ZPUI ($ExistingVersion).$\r$\n$\r$\nБудет выполнено обновление до версии ${VERSION}.$\r$\nВаши настройки и данные будут сохранены.$\r$\n$\r$\nНажмите OK для продолжения или «Отмена» для отказа."

LangString MsgSameVer ${LANG_ENGLISH} "ZPUI version $ExistingVersion is already installed.$\r$\n$\r$\nDo you want to reinstall it?$\r$\n$\r$\nClick OK to continue or Cancel to abort."
LangString MsgSameVer ${LANG_RUSSIAN} "ZPUI версии $ExistingVersion уже установлен.$\r$\n$\r$\nПереустановить?$\r$\n$\r$\nНажмите OK для продолжения или «Отмена» для отказа."

LangString MsgDowngrade ${LANG_ENGLISH} "A newer version of ZPUI ($ExistingVersion) is already installed.$\r$\n$\r$\nIt is not recommended to install an older version (${VERSION}).$\r$\n$\r$\nDo you want to continue anyway?"
LangString MsgDowngrade ${LANG_RUSSIAN} "Установлена более новая версия ZPUI ($ExistingVersion).$\r$\n$\r$\nНе рекомендуется устанавливать более старую версию (${VERSION}).$\r$\n$\r$\nПродолжить в любом случае?"

LangString MsgRemoveZapret ${LANG_ENGLISH} "Remove the Zapret DPI engine and its configuration?$\r$\n$\r$\nIf you plan to reinstall ZPUI later, you can keep it."
LangString MsgRemoveZapret ${LANG_RUSSIAN} "Удалить движок Zapret и его конфигурацию?$\r$\n$\r$\nЕсли вы планируете переустановить ZPUI позже, можете его оставить."

LangString DESC_SecCore ${LANG_ENGLISH} "ZPUI core application, satellite tools and mods."
LangString DESC_SecCore ${LANG_RUSSIAN} "Основное приложение ZPUI, спутники и моды."
LangString DESC_SecStartMenu ${LANG_ENGLISH} "Create a shortcut in the Start Menu."
LangString DESC_SecStartMenu ${LANG_RUSSIAN} "Создать ярлык в меню «Пуск»."
LangString DESC_SecDesktop ${LANG_ENGLISH} "Create a shortcut on the Desktop."
LangString DESC_SecDesktop ${LANG_RUSSIAN} "Создать ярлык на рабочем столе."

; ==============================================================================
;  Sections
; ==============================================================================
Section "ZPUI" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"

  ; Ensure ZPUI is closed (file-lock check — works regardless of privileges)
  Call EnsureAppClosed

  ; Write all dist files (overwrites binaries, preserves user data)
  File /r "${DIST}\*.*"

  ; Store install dir + version
  WriteRegStr HKCU "Software\ZPUI" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\ZPUI" "Version" "${VERSION}"

  ; Uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayName" "ZPUI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayIcon" "$INSTDIR\zpui.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "Publisher" "SuzucaRU"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "URLInfoAbout" "https://github.com/suzcuaru/ZPUI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "URLUpdateInfo" "https://github.com/suzcuaru/ZPUI/releases/latest"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoRepair" 1

  ; Estimated size
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "EstimatedSize" "$0"
SectionEnd

Section "Start Menu shortcut" SecStartMenu
  CreateDirectory "$SMPROGRAMS\ZPUI"
  CreateShortcut "$SMPROGRAMS\ZPUI\ZPUI.lnk" "$INSTDIR\zpui.exe" "" "$INSTDIR\zpui.exe"
  CreateShortcut "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\ZPUI.lnk" "$INSTDIR\zpui.exe" "" "$INSTDIR\zpui.exe"
SectionEnd

; ==============================================================================
;  Section descriptions (must come after sections)
; ==============================================================================
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} $(DESC_SecStartMenu)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ==============================================================================
;  Helper: ensure ZPUI is closed via file-lock detection
;  Works regardless of privilege level (installer is per-user, ZPUI may be elevated)
; ==============================================================================
Function EnsureAppClosed
  IfFileExists "$INSTDIR\zpui.exe" 0 done
  retry:
    ClearErrors
    FileOpen $0 "$INSTDIR\zpui.exe" a
    IfErrors 0 close_ok
      MessageBox MB_RETRYCANCEL|MB_ICONSTOP "$(MsgAppRunning)" IDRETRY retry
      Quit
    close_ok:
    FileClose $0
  done:
FunctionEnd

; ==============================================================================
;  Helper: skip directory page on upgrade/same/downgrade
; ==============================================================================
Function SkipDirectoryPage
  ${If} $UpgradeMode > 0
    Abort
  ${EndIf}
FunctionEnd

; ==============================================================================
;  .onInit — detect existing installation, compare versions
; ==============================================================================
Function .onInit
  StrCpy $UpgradeMode 0
  StrCpy $ExistingVersion ""

  ReadRegStr $ExistingVersion HKCU "Software\ZPUI" "Version"
  ReadRegStr $ExistingDir    HKCU "Software\ZPUI" "InstallDir"

  ${If} $ExistingVersion != ""
    ${VersionCompare} "${VERSION}" "$ExistingVersion" $R0

    ${If} $R0 == 1
      StrCpy $UpgradeMode 1
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_OKCANCEL|MB_ICONINFORMATION "$(MsgUpgrade)" IDOK +2
      Abort

    ${ElseIf} $R0 == 0
      StrCpy $UpgradeMode 2
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_OKCANCEL|MB_ICONQUESTION "$(MsgSameVer)" IDOK +2
      Abort

    ${Else}
      StrCpy $UpgradeMode 3
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_YESNO|MB_ICONEXCLAMATION "$(MsgDowngrade)" IDNO +2
      Abort
    ${EndIf}
  ${EndIf}

  ; Default: shortcuts checked
  SectionSetFlags ${SecStartMenu} 1
  SectionSetFlags ${SecDesktop} 1

  ; On upgrade, pre-check shortcuts based on existing ones
  ${If} $UpgradeMode > 0
    ${IfNot} ${FileExists} "$SMPROGRAMS\ZPUI\ZPUI.lnk"
      SectionSetFlags ${SecStartMenu} 0
    ${EndIf}
    ${IfNot} ${FileExists} "$DESKTOP\ZPUI.lnk"
      SectionSetFlags ${SecDesktop} 0
    ${EndIf}
  ${EndIf}
FunctionEnd

; ==============================================================================
;  un.onInit — restore language for uninstaller
; ==============================================================================
Function un.onInit
  !insertmacro MUI_UNGETLANGUAGE
FunctionEnd

; ==============================================================================
;  Uninstall
; ==============================================================================
Section "Uninstall"
  ; Stop running app
  nsExec::ExecToLog 'taskkill /IM zpui.exe /F'
  Sleep 1000

  ; --- Application binaries ---
  Delete "$INSTDIR\zpui.exe"
  Delete "$INSTDIR\wizard.exe"
  Delete "$INSTDIR\autoselect.exe"
  Delete "$INSTDIR\selfupdate.exe"
  Delete "$INSTDIR\zapretupdate.exe"
  Delete "$INSTDIR\versions.json"
  Delete "$INSTDIR\uninstall.exe"

  ; --- Zapret (ask) ---
  ${If} ${FileExists} "$INSTDIR\zapret"
    MessageBox MB_YESNO|MB_ICONQUESTION "$(MsgRemoveZapret)" IDNO skip_zapret
    RMDir /r "$INSTDIR\zapret"
  ${EndIf}
  skip_zapret:

  ; --- Mods ---
  RMDir /r "$INSTDIR\mods"

  ; --- Backups ---
  RMDir /r "$INSTDIR\backups"
  RMDir /r "$INSTDIR\.backup"

  ; --- Logs ---
  RMDir /r "$INSTDIR\logs"

  ; --- Database + Config ---
  Delete "$INSTDIR\zpui.db"
  Delete "$INSTDIR\config.json"

  ; Remove install dir if empty
  RMDir "$INSTDIR"

  ; Shortcuts
  Delete "$SMPROGRAMS\ZPUI\ZPUI.lnk"
  Delete "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk"
  RMDir "$SMPROGRAMS\ZPUI"
  Delete "$DESKTOP\ZPUI.lnk"

  ; Registry
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI"
  DeleteRegKey HKCU "Software\ZPUI"
SectionEnd
