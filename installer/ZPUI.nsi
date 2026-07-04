; ZPUI installer - per-user, no admin required (enables in-app self-update)
; Build: makensis /DVERSION=1.0.46 /DDIST=build\dist installer\ZPUI.nsi

!include "MUI2.nsh"
!include "LogicLib.nsh"

!ifndef VERSION
  !define VERSION "1.0.0"
!endif
!ifndef DIST
  !define DIST "build\dist"
!endif
!ifndef ICON
  !define ICON "build\windows\icon.ico"
!endif

Name "ZPUI"
OutFile "build\ZPUI-Setup-${VERSION}.exe"
Unicode True
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\ZPUI"
InstallDirRegKey HKCU "Software\ZPUI" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show
SetCompressor /SOLID lzma

; Version info embedded in the .exe
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "ZPUI"
VIAddVersionKey "FileDescription" "ZPUI - Zapret DPI bypass controller"
VIAddVersionKey "CompanyName" "ZPUI"
VIAddVersionKey "LegalCopyright" "ZPUI"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"

; --- Modern UI ---
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\zpui.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ZPUI"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Russian"

; --- Install section ---
Section "ZPUI" SecCore
  SectionIn RO
  SetOutPath "$INSTDIR"
  File /r "${DIST}\*.*"

  ; Store install dir
  WriteRegStr HKCU "Software\ZPUI" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\ZPUI" "Version" "${VERSION}"

  ; Uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Add/Remove Programs entry (per-user)
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayName" "ZPUI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "DisplayIcon" "$INSTDIR\zpui.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "Publisher" "ZPUI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "URLInfoAbout" "https://github.com/suzcuaru/ZPUI"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\ZPUI" "NoRepair" 1
SectionEnd

Section "Start Menu shortcut" SecStartMenu
  CreateDirectory "$SMPROGRAMS\ZPUI"
  CreateShortcut "$SMPROGRAMS\ZPUI\ZPUI.lnk" "$INSTDIR\zpui.exe" "" "$INSTDIR\zpui.exe"
  CreateShortcut "$SMPROGRAMS\ZPUI\Uninstall ZPUI.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\ZPUI.lnk" "$INSTDIR\zpui.exe" "" "$INSTDIR\zpui.exe"
SectionEnd

; --- Descriptions ---
LangString DESC_SecCore ${LANG_ENGLISH} "ZPUI core application and tools."
LangString DESC_SecCore ${LANG_RUSSIAN} "Основное приложение ZPUI и инструменты."
LangString DESC_SecStartMenu ${LANG_ENGLISH} "Create a Start Menu shortcut."
LangString DESC_SecStartMenu ${LANG_RUSSIAN} "Создать ярлык в меню «Пуск»."
LangString DESC_SecDesktop ${LANG_ENGLISH} "Create a Desktop shortcut."
LangString DESC_SecDesktop ${LANG_RUSSIAN} "Создать ярлык на рабочем столе."

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
!insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
!insertmacro MUI_DESCRIPTION_TEXT ${SecStartMenu} $(DESC_SecStartMenu)
!insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Function .onInit
  ; Default: shortcuts checked
  SectionSetFlags ${SecStartMenu} 1
  SectionSetFlags ${SecDesktop} 1
FunctionEnd

; --- Uninstall ---
Section "Uninstall"
  ; Stop running app
  nsExec::ExecToLog 'taskkill /IM zpui.exe /F'
  Sleep 1000

  ; Remove files (installed set + runtime artifacts)
  Delete "$INSTDIR\zpui.exe"
  Delete "$INSTDIR\wizard.exe"
  Delete "$INSTDIR\autoselect.exe"
  Delete "$INSTDIR\selfupdate.exe"
  Delete "$INSTDIR\zapretupdate.exe"
  Delete "$INSTDIR\versions.json"
  Delete "$INSTDIR\uninstall.exe"
  RMDir /r "$INSTDIR\mods"
  RMDir /r "$INSTDIR\backups"
  RMDir /r "$INSTDIR\.backup"
  RMDir /r "$INSTDIR\logs"
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
