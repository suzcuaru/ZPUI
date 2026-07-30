; ==============================================================================
;  ZPUI Installer (NSIS) — redesigned as Update Center
;  Per-user, no admin required (enables in-app self-update)
;  Bilingual: auto-detects Windows language (Russian / English)
;
;  Features:
;    - MIT License agreement page
;    - Smart version detection: upgrade / reinstall / downgrade
;    - Branch detection: stable vs dev, with warning for dev
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
!include "nsDialogs.nsh"

; --- Configurable defines ---
!ifndef VERSION
  !define VERSION "1.0.0"
!endif
!ifndef ARCH
  !define ARCH "win32"
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
Var CleanInstallMode     ; 0=normal, 1=clean (with cache clear)
Var InstallModeRadio     ; 0=update, 1=clean
Var Dialog_Hwnd

; Dev warning variables
Var IsDevVersion
Var DevConfirmCheckbox

; ==============================================================================
;  General settings
; ==============================================================================
Name "ZPUI ${VERSION}"
OutFile "${OUTDIR}\ZPUI-Setup-${VERSION}-${ARCH}.exe"
Unicode True
RequestExecutionLevel admin
InstallDir "$PROGRAMFILES\ZPUI"
InstallDirRegKey HKLM "Software\ZPUI" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma

BrandingText "ZPUI ${VERSION}  ·  github.com/suzcuaru/ZPUI"

; --- Version info ---
!ifndef VERSION_NUM
  !define VERSION_NUM "${VERSION}"
!endif
; Ensure 4-part version for VIProductVersion (X.X.X.X)
!searchparse /noerrors "${VERSION_NUM}" "" _vi1 "." _vi2 "." _vi3 "." _vi4
!ifndef _vi1
  !define _vi1 "0"
!endif
!ifndef _vi2
  !define _vi2 "0"
!endif
!ifndef _vi3
  !define _vi3 "0"
!endif
!ifndef _vi4
  !define _vi4 "0"
!endif
!define VER_4PART "${_vi1}.${_vi2}.${_vi3}.${_vi4}"
VIProductVersion "${VER_4PART}"
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

; --- Finish page ---
!define MUI_FINISHPAGE_RUN "$INSTDIR\ZPUI.exe"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.md"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!define MUI_FINISHPAGE_LINK "github.com/suzcuaru/ZPUI"
!define MUI_FINISHPAGE_LINK_LOCATION "https://github.com/suzcuaru/ZPUI"

; ==============================================================================
;  Pages — install
; ==============================================================================
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE}"
Page custom fnc_DevWarning_Show fnc_DevWarning_Leave   ; NEW: dev warning
Page custom fnc_InstallMode_Show fnc_InstallMode_Leave
!insertmacro MUI_PAGE_COMPONENTS
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

LangString MsgRemoveZapret ${LANG_ENGLISH} "Remove the Zapret DPI engine and its configuration?$\r$\n$\r$\nIf you plan to reinstall ZPUI later, you can keep it."
LangString MsgRemoveZapret ${LANG_RUSSIAN} "Удалить движок Zapret и его конфигурацию?$\r$\n$\r$\nЕсли вы планируете переустановить ZPUI позже, можете его оставить."

LangString DESC_SecCore ${LANG_ENGLISH} "ZPUI core application, satellite tools and mods."
LangString DESC_SecCore ${LANG_RUSSIAN} "Основное приложение ZPUI, спутники и моды."
LangString DESC_SecStartMenu ${LANG_ENGLISH} "Create a shortcut in the Start Menu."
LangString DESC_SecStartMenu ${LANG_RUSSIAN} "Создать ярлык в меню «Пуск»."
LangString DESC_SecDesktop ${LANG_ENGLISH} "Create a shortcut on the Desktop."
LangString DESC_SecDesktop ${LANG_RUSSIAN} "Создать ярлык на рабочем столе."

; Install mode page strings
LangString InstModeTitle ${LANG_ENGLISH} "Installation Mode"
LangString InstModeTitle ${LANG_RUSSIAN} "Режим установки"
LangString InstModeSubtitle ${LANG_ENGLISH} "ZPUI $ExistingVersion is already installed."
LangString InstModeSubtitle ${LANG_RUSSIAN} "ZPUI версии $ExistingVersion уже установлена."
LangString InstModeUpdate ${LANG_ENGLISH} "Update (keep settings and data)"
LangString InstModeUpdate ${LANG_RUSSIAN} "Обновить (сохранить настройки и данные)"
LangString InstModeUpdateDesc ${LANG_ENGLISH} "Overwrite application files. Your settings, database and logs will be preserved."
LangString InstModeUpdateDesc ${LANG_RUSSIAN} "Заменить файлы приложения. Ваши настройки, база данных и логи будут сохранены."
LangString InstModeClean ${LANG_ENGLISH} "Clean install (remove all data)"
LangString InstModeClean ${LANG_RUSSIAN} "Чистая установка (удалить все данные)"
LangString InstModeCleanDesc ${LANG_ENGLISH} "Completely remove ZPUI: all files, settings, database, logs, shortcuts and registry.$\r$\nNothing will be left."
LangString InstModeCleanDesc ${LANG_RUSSIAN} "Полностью удалить ZPUI: все файлы, настройки, базу данных, логи, ярлыки и реестр.$\r$\nНичего не останется."

; Dev warning strings
LangString DevWarnTitle ${LANG_ENGLISH} "Development Version Warning"
LangString DevWarnTitle ${LANG_RUSSIAN} "Предупреждение: Dev-версия"
LangString DevWarnText ${LANG_ENGLISH} "You are installing a DEVELOPMENT version of ZPUI.$\r$\n$\r$\nDev versions contain the latest changes but may be unstable, have bugs, or incomplete features.$\r$\n$\r$\nFor stable daily use, switch to the stable branch."
LangString DevWarnText ${LANG_RUSSIAN} "Вы устанавливаете DEVELOPMENT версию ZPUI.$\r$\n$\r$\nDev-версии содержат последние изменения, но могут быть нестабильными, содержать ошибки или недоделанные функции.$\r$\n$\r$\nДля стабильной ежедневной работы переключитесь на stable-ветку."
LangString DevWarnConfirm ${LANG_ENGLISH} "I understand, install anyway"
LangString DevWarnConfirm ${LANG_RUSSIAN} "Я понимаю, всё равно установить"
LangString DevWarnMustConfirm ${LANG_ENGLISH} "You must confirm that you understand the risks to continue."
LangString DevWarnMustConfirm ${LANG_RUSSIAN} "Подтвердите, что вы понимаете риски, чтобы продолжить."


; ==============================================================================
;  Dev Warning custom page
; ==============================================================================
Function fnc_DevWarning_Show
  ; Only show if the version being installed is a dev version (4+ segments)
  StrCpy $IsDevVersion 0
  Push "${VERSION}"
  Call fnc_IsDevVersion
  Pop $IsDevVersion

  ${If} $IsDevVersion == 0
    Abort  ; skip page for stable versions
  ${EndIf}

  nsDialogs::Create 1018
  Pop $Dialog_Hwnd

  ${If} $Dialog_Hwnd == error
    Abort
  ${EndIf}

  ; Title
  ${NSD_CreateLabel} 0 0 100% 24u "$(DevWarnTitle)"
  Pop $0
  CreateFont $1 "Segoe UI" 12 700
  SendMessage $0 ${WM_SETFONT} $1 0

  ; Warning icon area (unicode ⚠)
  ${NSD_CreateLabel} 0 30u 100% 80u "$(DevWarnText)"
  Pop $0
  CreateFont $1 "Segoe UI" 10 400
  SendMessage $0 ${WM_SETFONT} $1 0

  ; Confirm checkbox
  ${NSD_CreateCheckBox} 0 120u 100% 14u "$(DevWarnConfirm)"
  Pop $DevConfirmCheckbox

  nsDialogs::Show
FunctionEnd

Function fnc_DevWarning_Leave
  ${NSD_GetState} $DevConfirmCheckbox $0
  ${If} $0 == 0
    MessageBox MB_ICONSTOP|MB_OK "$(DevWarnMustConfirm)"
    Abort
  ${EndIf}
FunctionEnd


; ==============================================================================
;  Install Mode custom page (modified — shows branch info)
; ==============================================================================
Function fnc_InstallMode_Show
  ${If} $UpgradeMode == 0
    Abort
  ${EndIf}

  nsDialogs::Create 1018
  Pop $Dialog_Hwnd

  ${If} $Dialog_Hwnd == error
    Abort
  ${EndIf}

  ; Title
  ${NSD_CreateLabel} 0 0 100% 20u "$(InstModeTitle)"
  Pop $0
  CreateFont $1 "Segoe UI" 12 700
  SendMessage $0 ${WM_SETFONT} $1 0

  ; Subtitle with version info and branch
  Push "$ExistingVersion"
  Call fnc_IsDevVersion
  Pop $0
  ${If} $0 == 1
    ${NSD_CreateLabel} 0 22u 100% 20u "$(InstModeSubtitle) [DEV]"
  ${Else}
    ${NSD_CreateLabel} 0 22u 100% 20u "$(InstModeSubtitle)"
  ${EndIf}
  Pop $0

  ; New version display with branch
  Push "${VERSION}"
  Call fnc_IsDevVersion
  Pop $0
  StrCpy $2 "New version: ${VERSION}"
  ${If} $0 == 1
    StrCpy $2 "$2 [DEV]"
  ${EndIf}
  ${NSD_CreateLabel} 0 38u 100% 14u "$2"
  Pop $0
  CreateFont $1 "Segoe UI" 9 400
  SendMessage $0 ${WM_SETFONT} $1 0
  SetCtlColors $0 "gray" transparent

  ; Radio: Update
  ${NSD_CreateRadioButton} 0 56u 100% 14u "$(InstModeUpdate)"
  Pop $InstallModeRadio

  ${NSD_CreateLabel} 16u 72u 95% 20u "$(InstModeUpdateDesc)"
  Pop $0
  SetCtlColors $0 "gray" transparent

  ; Radio: Clean install
  ${NSD_CreateRadioButton} 0 98u 100% 14u "$(InstModeClean)"
  Pop $0

  ${NSD_CreateLabel} 16u 114u 95% 28u "$(InstModeCleanDesc)"
  Pop $0
  SetCtlColors $0 "gray" transparent

  ; Default: update selected
  SendMessage $InstallModeRadio ${BM_SETCHECK} ${BST_CHECKED} 0

  nsDialogs::Show
FunctionEnd

Function fnc_InstallMode_Leave
  ${NSD_GetState} $InstallModeRadio $0

  ${If} $0 == ${BST_CHECKED}
    StrCpy $CleanInstallMode 0
  ${Else}
    StrCpy $CleanInstallMode 1
  ${EndIf}
FunctionEnd

; ==============================================================================
;  Sections
; ==============================================================================
Section "ZPUI" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"

  ; Ensure ZPUI is closed (file-lock check — works regardless of privileges)
  Call EnsureAppClosed

  ; Clean install mode: complete removal
  ${If} $CleanInstallMode == 1
    nsExec::ExecToLog 'taskkill /IM ZPUI.exe /F'
    Sleep 500
    RMDir /r "$INSTDIR"
    RMDir /r "$APPDATA\ZPUI"
    RMDir /r "$LOCALAPPDATA\ZPUI"
    RMDir /r "$LOCALAPPDATA\Microsoft\EdgeWebView"
    RMDir /r "$LOCALAPPDATA\Microsoft\WebView2"
    Delete "$SMPROGRAMS\ZPUI\ZPUI.lnk"
    Delete "$SMPROGRAMS\ZPUI\ЗАПРЕТ.lnk"
    Delete "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk"
    RMDir "$SMPROGRAMS\ZPUI"
    Delete "$DESKTOP\ZPUI.lnk"
    Delete "$DESKTOP\ЗАПРЕТ.lnk"
    DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI"
    DeleteRegKey HKLM "Software\ZPUI"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "ZPUI"
    nsExec::ExecToLog 'schtasks /delete /tn "ZPUI" /f'
  ${EndIf}

  ; Write all dist files
  ; First pass: copy everything except potentially locked zapret DLLs/exes
  ; /x "components" — папка components/ относится к распределению Яндекс.Диска,
  ; в установку она не попадает (модули лежат плоско рядом с ZPUI.exe).
  ; /x "zapret" — копируется отдельно вторым проходом с SetOverwrite off.
  File /r /x "zpui.db" /x "*.db-journal" /x "*.db-wal" /x "*.db-shm" \
             /x "config.json" /x "logs" /x ".backup" \
             /x "*.zip" /x "*.old" /x ".last_update" /x "gh_release_cache.json" \
             /x "WinDivert.dll" /x "WinDivert64.sys" /x "winws.exe" \
             /x "components" /x "zapret" \
             "${DIST}\*.*"

  ; Remove legacy components/ folder from previous installs (now flat layout)
  RMDir /r "$INSTDIR\components"

  ; Second pass: copy zapret binaries with Overwrite off (locked files skipped silently)
  ; If zapret files don't exist or are locked, this step is skipped gracefully.
  !ifdef ZAPRET_OK
  SetOverwrite off
  SetOutPath "$INSTDIR\zapret"
  File /r /x "version.txt" /x "checksum.sha256" "${DIST}\zapret\*.*"
  SetOutPath "$INSTDIR"
  SetOverwrite on
  !endif

  ; Include README.md for the finish page (file is next to installer\ folder)
  File "..\README.md"

  ; Store install dir + version
  WriteRegStr HKLM "Software\ZPUI" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\ZPUI" "Version" "${VERSION}"

  ; Uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayName" "ZPUI"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayIcon" "$INSTDIR\ZPUI.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "Publisher" "SuzucaRU"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "URLInfoAbout" "https://github.com/suzcuaru/ZPUI"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "URLUpdateInfo" "https://github.com/suzcuaru/ZPUI/releases/latest"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoRepair" 1

  ; Estimated size
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "EstimatedSize" "$0"
SectionEnd

Section "Start Menu shortcut" SecStartMenu
  CreateDirectory "$SMPROGRAMS\ZPUI"
  CreateShortcut "$SMPROGRAMS\ZPUI\ЗАПРЕТ.lnk" "$INSTDIR\ZPUI.exe" "" "$INSTDIR\ZPUI.exe" 0 "" "" "ZPUI — Zapret DPI bypass controller"
  CreateShortcut "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\ЗАПРЕТ.lnk" "$INSTDIR\ZPUI.exe" "" "$INSTDIR\ZPUI.exe" 0 "" "" "ZPUI — Zapret DPI bypass controller"
SectionEnd

; ==============================================================================
;  Section descriptions
; ==============================================================================
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} $(DESC_SecStartMenu)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ==============================================================================
;  Helper: ensure ZPUI is closed via file-lock detection
; ==============================================================================
Function EnsureAppClosed
  IfFileExists "$INSTDIR\ZPUI.exe" 0 done
  retry:
    ClearErrors
    FileOpen $0 "$INSTDIR\ZPUI.exe" a
    IfErrors 0 close_ok
      MessageBox MB_RETRYCANCEL|MB_ICONSTOP "$(MsgAppRunning)" IDRETRY retry
      Quit
    close_ok:
    FileClose $0
  done:
FunctionEnd


; ==============================================================================
;  Helper: check if version is dev (4+ segments, last != 0)
;  Stable: 3 segments (1.5.5) or 4 segments ending with 0 (1.5.5.0)
;  Dev:    4+ segments with last != 0 (1.5.5.1, 1.5.5.2)
; ==============================================================================
Function fnc_IsDevVersion
  Pop $0           ; version string
  
  StrLen $1 $0
  ${If} $1 == 0
    Push 0
    return
  ${EndIf}
  
  ; Count dots
  Push $0
  Push "."
  Call fnc_StrCount
  Pop $1
  
  ; < 4 segments → stable
  ${If} $1 < 3
    Push 0
    return
  ${EndIf}
  
  ; 4+ segments — extract last segment
  StrCpy $2 $0
  find_loop:
    Push $2
    Push "."
    Call fnc_StrStr
    Pop $0
    ${If} $0 == ""
      goto check_dev
    ${EndIf}
    StrCpy $2 $0 "" 1
    goto find_loop
  
  check_dev:
  ${If} $2 == "0"
    Push 0  ; last segment = 0 → stable
  ${Else}
    Push 1  ; last segment ≠ 0 → dev
  ${EndIf}
FunctionEnd

; ==============================================================================
;  Helper: count occurrences of a substring
; ==============================================================================
Function fnc_StrCount
  Exch $0  ; substring
  Exch
  Exch $1  ; string
  Push $2
  Push $3
  Push $4
  
  StrCpy $2 0
  StrLen $3 $0
  ${If} $3 == 0
    goto done
  ${EndIf}
  
  loop:
    StrLen $4 $1
    ${If} $4 == 0
      goto done
    ${EndIf}
    Push $1
    Push $0
    Call fnc_StrStr
    Pop $4
    ${If} $4 == ""
      goto done
    ${EndIf}
    IntOp $2 $2 + 1
    StrCpy $1 $4 "" $3
    goto loop
  
  done:
  Pop $4
  Pop $3
  Pop $1
  Pop $0
  Exch $2
FunctionEnd

; ==============================================================================
;  Helper: find substring position
; ==============================================================================
Function fnc_StrStr
  Exch $0  ; needle
  Exch
  Exch $1  ; haystack
  Push $2
  Push $3
  Push $4
  
  StrCpy $2 0
  StrLen $3 $0
  ${If} $3 == 0
    goto done
  ${EndIf}
  
  loop:
    StrCpy $4 $1 $3 $2
    ${If} $4 == ""
      StrCpy $0 ""
      goto done
    ${EndIf}
    ${If} $4 == $0
      StrCpy $0 $1 "" $2
      goto done
    ${EndIf}
    IntOp $2 $2 + 1
    goto loop
  
  done:
  Pop $4
  Pop $3
  Pop $1
  Exch $0
FunctionEnd



; ==============================================================================
;  .onInit — detect existing installation, compare versions, init vars
; ==============================================================================
Function .onInit
  StrCpy $UpgradeMode 0
  StrCpy $ExistingVersion ""
  StrCpy $CleanInstallMode 0

  ReadRegStr $ExistingVersion HKLM "Software\ZPUI" "Version"
  StrCpy $INSTDIR "$PROGRAMFILES\ZPUI"

  ${If} $ExistingVersion != ""
    ${VersionCompare} "${VERSION}" "$ExistingVersion" $R0

    ${If} $R0 == 1
      StrCpy $UpgradeMode 1   ; upgrade
    ${ElseIf} $R0 == 0
      StrCpy $UpgradeMode 2   ; same version
    ${Else}
      StrCpy $UpgradeMode 3   ; downgrade
    ${EndIf}
  ${EndIf}

  ; Default: shortcuts checked
  SectionSetFlags ${SecStartMenu} 1
  SectionSetFlags ${SecDesktop} 1

  ; On upgrade, pre-check shortcuts based on existing ones
  ${If} $UpgradeMode > 0
    ${IfNot} ${FileExists} "$SMPROGRAMS\ZPUI\ZPUI.lnk"
    ${AndIfNot} ${FileExists} "$SMPROGRAMS\ZPUI\ЗАПРЕТ.lnk"
      SectionSetFlags ${SecStartMenu} 0
    ${EndIf}
    ${IfNot} ${FileExists} "$DESKTOP\ZPUI.lnk"
    ${AndIfNot} ${FileExists} "$DESKTOP\ЗАПРЕТ.lnk"
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
;  Uninstall — comprehensive cleanup
; ==============================================================================
Section "Uninstall"
  ; Stop running app
  nsExec::ExecToLog 'taskkill /IM ZPUI.exe /F'
  Sleep 1000

  ; Kill satellite processes
  nsExec::ExecToLog 'taskkill /IM selfupdate.exe /F'
  nsExec::ExecToLog 'taskkill /IM report.exe /F'
  nsExec::ExecToLog 'taskkill /IM security.exe /F'

  ; Stop Zapret service if running
  nsExec::ExecToLog 'net stop "ZPUI" /Y'
  Sleep 500

  ; --- Application binaries ---
  Delete "$INSTDIR\ZPUI.exe"
  Delete "$INSTDIR\selfupdate.exe"
  Delete "$INSTDIR\report.exe"
  Delete "$INSTDIR\security.exe"
  Delete "$INSTDIR\wizard.exe"
  Delete "$INSTDIR\uninstall.exe"

  ; --- Manifest / config files ---
  Delete "$INSTDIR\checksums.sha256"
  Delete "$INSTDIR\versions.json"
  Delete "$INSTDIR\version.txt"
  Delete "$INSTDIR\README.md"

  ; --- Components (all modules in canonical location) ---
  RMDir /r "$INSTDIR\components"

  ; --- Zapret (always remove completely) ---
  RMDir /r "$INSTDIR\zapret"

  ; --- Database directory ---
  RMDir /r "$INSTDIR\databases"

  ; --- Mods ---
  RMDir /r "$INSTDIR\mods"

  ; --- Backups ---
  RMDir /r "$INSTDIR\backups"
  RMDir /r "$INSTDIR\.backup"

  ; --- Logs ---
  RMDir /r "$INSTDIR\logs"

  ; --- Config + legacy DB ---
  Delete "$INSTDIR\zpui.db"
  Delete "$INSTDIR\config.json"

  ; Remove install dir if empty
  RMDir "$INSTDIR"

  ; --- User data directories ---
  RMDir /r "$APPDATA\ZPUI"
  RMDir /r "$LOCALAPPDATA\ZPUI"

  ; --- WebView2 cache ---
  RMDir /r "$LOCALAPPDATA\Microsoft\EdgeWebView"
  RMDir /r "$LOCALAPPDATA\Microsoft\WebView2"

  ; Shortcuts
  Delete "$SMPROGRAMS\ZPUI\ZPUI.lnk"
  Delete "$SMPROGRAMS\ZPUI\ЗАПРЕТ.lnk"
  Delete "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk"
  RMDir "$SMPROGRAMS\ZPUI"
  Delete "$DESKTOP\ZPUI.lnk"
  Delete "$DESKTOP\ЗАПРЕТ.lnk"

  ; Registry
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI"
  DeleteRegKey HKLM "Software\ZPUI"

  ; Autostart registry key + scheduled task
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "ZPUI"
  nsExec::ExecToLog 'schtasks /delete /tn "ZPUI" /f'

  ; Remove Zapret service if installed separately
  nsExec::ExecToLog 'sc delete "zpui_service"'
  nsExec::ExecToLog 'sc delete "ZPUI"'
SectionEnd
