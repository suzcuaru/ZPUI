; ==============================================================================
;  ZPUI Installer (NSIS)
;  Per-user, no admin required (enables in-app self-update)
;
;  Features:
;    - MIT License agreement page
;    - Smart version detection: upgrade / reinstall / downgrade detection
;    - Preserves user data (config, logs, zapret, backups) on update
;    - Auto-kills running ZPUI before overwrite
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
!ifndef DIST
  !define DIST "build\dist"
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
Var ExistingVersion      ; version string of the installed copy ("1.0.48")
Var ExistingDir          ; install dir of the installed copy
Var UpgradeMode          ; 0 = fresh install
                          ; 1 = upgrade   (installed < this)
                          ; 2 = same       (installed == this)
                          ; 3 = downgrade  (installed > this)

; ==============================================================================
;  General settings
; ==============================================================================
Name "ZPUI"
OutFile "${OUTDIR}\ZPUI-Setup-${VERSION}.exe"
Unicode True
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\ZPUI"
InstallDirRegKey HKCU "Software\ZPUI" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma

BrandingText "ZPUI ${VERSION}  ·  github.com/suzcuaru/ZPUI"

; --- Version info embedded in the .exe ---
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "ZPUI"
VIAddVersionKey "FileDescription" "ZPUI — Zapret DPI bypass controller"
VIAddVersionKey "CompanyName" "SuzucaRU"
VIAddVersionKey "LegalCopyright" "Copyright (c) 2026 SuzucaRU — MIT License"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

; ==============================================================================
;  Modern UI — appearance
; ==============================================================================
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING

; --- Welcome page ---
!define MUI_WELCOMEPAGE_TITLE "Welcome to the ZPUI ${VERSION} Setup Wizard"
!define MUI_WELCOMEPAGE_TEXT "This wizard will guide you through the installation of ZPUI ${VERSION}.$\r$\n$\r$\nZPUI is a Windows GUI controller for Zapret (DPI bypass), with built-in proxy, traffic monitoring, Xbox DNS configuration and auto-update.$\r$\n$\r$\nIt is recommended to close all other applications before continuing.$\r$\n$\r$\nClick Next to continue."

; --- License page ---
!define MUI_LICENSEPAGE_TEXT_TOP "Please review the license terms before installing ZPUI."
!define MUI_LICENSEPAGE_TEXT_BOTTOM "If you accept the terms of the agreement, click I Agree to continue."

; --- Finish page ---
!define MUI_FINISHPAGE_RUN "$INSTDIR\zpui.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ZPUI now"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!define MUI_FINISHPAGE_LINK "Visit ZPUI on GitHub"
!define MUI_FINISHPAGE_LINK_LOCATION "https://github.com/suzcuaru/ZPUI"

; ==============================================================================
;  Pages — install
; ==============================================================================
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${LICENSE}"
!insertmacro MUI_PAGE_COMPONENTS

; Skip directory page on upgrade / same / downgrade (keep existing dir)
!define MUI_PAGE_CUSTOMFUNCTION_PRE SkipDirectoryPage
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

; --- Pages — uninstall ---
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

; ==============================================================================
;  Languages
; ==============================================================================
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Russian"

; ==============================================================================
;  Install sections
; ==============================================================================
Section "ZPUI" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"

  ; Kill running instances before overwriting (safe during upgrades)
  Call KillRunningApp

  ; Write all dist files (overwrites binaries, preserves user data)
  File /r "${DIST}\*.*"

  ; Store install dir + version
  WriteRegStr HKCU "Software\ZPUI" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\ZPUI" "Version" "${VERSION}"

  ; Uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry (per-user)
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

  ; Estimated size for "Add/Remove Programs"
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
;  Descriptions
; ==============================================================================
LangString DESC_SecCore ${LANG_ENGLISH} "ZPUI core application, satellite tools and mods."
LangString DESC_SecCore ${LANG_RUSSIAN} "Основное приложение ZPUI, спутники и моды."
LangString DESC_SecStartMenu ${LANG_ENGLISH} "Create a shortcut in the Start Menu."
LangString DESC_SecStartMenu ${LANG_RUSSIAN} "Создать ярлык в меню «Пуск»."
LangString DESC_SecDesktop ${LANG_ENGLISH} "Create a shortcut on the Desktop."
LangString DESC_SecDesktop ${LANG_RUSSIAN} "Создать ярлык на рабочем столе."

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} $(DESC_SecStartMenu)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ==============================================================================
;  Helper: kill running ZPUI + satellites
; ==============================================================================
Function KillRunningApp
  nsExec::ExecToLog 'taskkill /IM zpui.exe /F'
  nsExec::ExecToLog 'taskkill /IM wizard.exe /F'
  nsExec::ExecToLog 'taskkill /IM autoselect.exe /F'
  nsExec::ExecToLog 'taskkill /IM selfupdate.exe /F'
  nsExec::ExecToLog 'taskkill /IM zapretupdate.exe /F'
  Sleep 500
FunctionEnd

; ==============================================================================
;  Helper: skip directory page when updating existing install
; ==============================================================================
Function SkipDirectoryPage
  ${If} $UpgradeMode > 0
    Abort
  ${EndIf}
FunctionEnd

; ==============================================================================
;  .onInit — detect existing installation and compare versions
; ==============================================================================
Function .onInit
  StrCpy $UpgradeMode 0
  StrCpy $ExistingVersion ""

  ; Read existing version and install dir from registry
  ReadRegStr $ExistingVersion HKCU "Software\ZPUI" "Version"
  ReadRegStr $ExistingDir    HKCU "Software\ZPUI" "InstallDir"

  ${If} $ExistingVersion != ""
    ; An installation was found — compare versions
    ${VersionCompare} "${VERSION}" "$ExistingVersion" $R0

    ${If} $R0 == 1
      ; --- Upgrade: installed version is older ---
      StrCpy $UpgradeMode 1
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_OKCANCEL|MB_ICONINFORMATION \
        "An older version of ZPUI ($ExistingVersion) is installed.$\r$\n$\r$\nThis will upgrade to version ${VERSION}.$\r$\nYour settings and data will be preserved.$\r$\n$\r$\nClick OK to continue or Cancel to abort." \
        IDOK +2
      Abort

    ${ElseIf} $R0 == 0
      ; --- Same version ---
      StrCpy $UpgradeMode 2
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_OKCANCEL|MB_ICONQUESTION \
        "ZPUI version $ExistingVersion is already installed.$\r$\n$\r$\nDo you want to reinstall it?$\r$\n$\r$\nClick OK to continue or Cancel to abort." \
        IDOK +2
      Abort

    ${Else}
      ; --- Downgrade: installed version is newer ($R0 == 2) ---
      StrCpy $UpgradeMode 3
      StrCpy $INSTDIR "$ExistingDir"
      MessageBox MB_YESNO|MB_ICONEXCLAMATION \
        "A newer version of ZPUI ($ExistingVersion) is already installed.$\r$\n$\r$\nIt is not recommended to install an older version (${VERSION}).$\r$\n$\r$\nDo you want to continue anyway?" \
        IDNO +2
      Abort
    ${EndIf}

    ; Kill running instances before proceeding with update
    Call KillRunningApp
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

  ; --- User data: ask before removing ---
  ; Zapret installation (downloaded by wizard — can be large)
  ${If} ${FileExists} "$INSTDIR\zapret"
    MessageBox MB_YESNO|MB_ICONQUESTION \
      "Remove the Zapret DPI engine and its configuration?$\r$\n$\r$\nIf you plan to reinstall ZPUI later, you can keep it." \
      IDNO skip_zapret
    RMDir /r "$INSTDIR\zapret"
  ${EndIf}
  skip_zapret:

  ; --- Mods (user customisation) ---
  RMDir /r "$INSTDIR\mods"

  ; --- Backups ---
  RMDir /r "$INSTDIR\backups"
  RMDir /r "$INSTDIR\.backup"

  ; --- Logs ---
  RMDir /r "$INSTDIR\logs"

  ; --- Database ---
  Delete "$INSTDIR\zpui.db"

  ; --- Config ---
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
