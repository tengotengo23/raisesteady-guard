; RaiseSteady Guard installer.
; Build: tools/build-installer.sh (makensis 3.x). Custom pages use
; nsDialogs, which ships with NSIS (zlib licence).
;
; Silent:    RaiseSteadyGuardSetup.exe /S /ACCOUNTS=all /BAND=9-12 /PIN=1234 [/DNS=off]
;            (/ACCOUNTS=S-1-5-21-...,S-1-5-21-... for chosen accounts)
; Uninstall: "C:\Program Files\RaiseSteadyGuard\uninstall.exe" /S /PIN=1234

Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
RequestExecutionLevel admin

!ifndef APP_NAME
  !define APP_NAME "RaiseSteady Guard"
!endif
!ifndef VERSION
  !define VERSION "0.4.0"
!endif
!ifndef DIST
  !define DIST "..\dist"
!endif
!ifndef OUTFILE
  !define OUTFILE "${DIST}\RaiseSteadyGuardSetup.exe"
!endif
!ifndef NOTICES
  !define NOTICES "..\THIRD-PARTY-NOTICES.txt"
!endif
!ifndef GO_NOTICE
  !define GO_NOTICE "..\agent\NOTICE-go.txt"
!endif
!if /FileExists "${NOTICES}"
!else
  !error "THIRD-PARTY-NOTICES.txt is missing. The installer must ship the licence notices."
!endif
!define PORT 47615
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\RaiseSteadyGuard"

Name "${APP_NAME}"
OutFile "${OUTFILE}"
InstallDir "$PROGRAMFILES64\RaiseSteadyGuard"
ShowInstDetails hide
ShowUninstDetails hide
BrandingText "${APP_NAME} ${VERSION}"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=0 "ProductName" "${APP_NAME}"
VIAddVersionKey /LANG=0 "FileDescription" "${APP_NAME} setup"
VIAddVersionKey /LANG=0 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=0 "LegalCopyright" "Nexodyne"

!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "x64.nsh"

Var Dialog
Var Accounts      ; "all" or SIDs joined by commas
Var Band
Var Pin
Var EveryoneBox
Var BandBox
Var Pin1Box
Var Pin2Box
Var UnPin
Var UnPinBox
Var Upgrade
Var DnsBox
Var FamilyDns   ; "on" or "off": AdGuard DNS Family for new accounts

!define MUI_ABORTWARNING
!define MUI_LANGDLL_REGISTRY_ROOT "HKLM"
!define MUI_LANGDLL_REGISTRY_KEY "${UNINST_KEY}"
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
!define MUI_ICON "${NSISDIR}\Contrib\Graphics\Icons\modern-install.ico"
!define MUI_UNICON "${NSISDIR}\Contrib\Graphics\Icons\modern-uninstall.ico"
!define MUI_WELCOMEPAGE_TITLE "$(WelcomeTitle)"
!define MUI_WELCOMEPAGE_TEXT "$(WelcomeText)"
!insertmacro MUI_PAGE_WELCOME
; Terms: no guarantees, no responsibility, privacy, licence (terms-<lang>.rtf; the parent clicks I Agree).
!insertmacro MUI_PAGE_LICENSE $(TermsFile)
Page custom AccountsPage AccountsLeave
Page custom AgePinPage AgePinLeave
Page custom DnsPage DnsLeave
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "$(FinishTitle)"
!define MUI_FINISHPAGE_TEXT "$(FinishText)"
!insertmacro MUI_PAGE_FINISH

UninstPage custom un.PinPage un.PinLeave
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Georgian"
!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_RESERVEFILE_LANGDLL

!include "strings.nsh"

; ---------------------------------------------------------------- helpers

; Extracts the agent for this processor to $0.
!macro PutAgent DEST
  ${If} ${IsNativeARM64}
    File "/oname=${DEST}" "${DIST}\raisesteady-guard-agent-arm64.exe"
  ${Else}
    File "/oname=${DEST}" "${DIST}\raisesteady-guard-agent-amd64.exe"
  ${EndIf}
!macroend

; $R0 = text; sets $R1 to 1 when it is 4 to 8 digits, else 0.
!macro CheckPinMacro UN
Function ${UN}CheckPin
  Push $2
  Push $3
  Push $4
  StrCpy $R1 0
  StrLen $2 $R0
  ${If} $2 < 4
  ${OrIf} $2 > 8
    Goto done
  ${EndIf}
  StrCpy $3 0
  ${Do}
    ${If} $3 >= $2
      StrCpy $R1 1
      ${Break}
    ${EndIf}
    StrCpy $4 $R0 1 $3
    ${If} $4 != "0"
    ${AndIf} $4 != "1"
    ${AndIf} $4 != "2"
    ${AndIf} $4 != "3"
    ${AndIf} $4 != "4"
    ${AndIf} $4 != "5"
    ${AndIf} $4 != "6"
    ${AndIf} $4 != "7"
    ${AndIf} $4 != "8"
    ${AndIf} $4 != "9"
      ${Break}
    ${EndIf}
    IntOp $3 $3 + 1
  ${Loop}
  done:
  Pop $4
  Pop $3
  Pop $2
FunctionEnd
!macroend
!insertmacro CheckPinMacro ""
!insertmacro CheckPinMacro "un."

; ---------------------------------------------------------------- install

Function .onInit
  ${IfNot} ${IsNativeAMD64}
  ${AndIfNot} ${IsNativeARM64}
    MessageBox MB_OK|MB_ICONSTOP "$(NeedsWin64)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
  SetRegView 64
  InitPluginsDir
  StrCpy $Upgrade 0
  ReadRegStr $0 HKLM "${UNINST_KEY}" "DisplayName"
  ${If} $0 != ""
    StrCpy $Upgrade 1
  ${EndIf}
  StrCpy $Band "9-12"
  StrCpy $FamilyDns "on"
  ${GetParameters} $R9
  ${If} ${Silent}
    ClearErrors
    ${GetOptions} $R9 "/DNS=" $0
    ${If} $0 == "off"
      StrCpy $FamilyDns "off"
    ${EndIf}
    ${GetOptions} $R9 "/ACCOUNTS=" $Accounts
    ${GetOptions} $R9 "/BAND=" $Band
    ${GetOptions} $R9 "/PIN=" $Pin
    StrCpy $R0 $Pin
    Call CheckPin
    ${If} $R1 != 1
    ${OrIf} $Accounts == ""
    ${OrIf} $Band == ""
      SetErrorLevel 2
      Abort
    ${EndIf}
  ${Else}
    !insertmacro MUI_LANGDLL_DISPLAY
  ${EndIf}
FunctionEnd

Function AccountsPage
  !insertmacro MUI_HEADER_TEXT "$(AccTitle)" "$(AccSub)"
  ${IfNot} ${FileExists} "$PLUGINSDIR\accounts.ini"
    !insertmacro PutAgent "$PLUGINSDIR\kga.exe"
    nsExec::ExecToStack '"$PLUGINSDIR\kga.exe" list-accounts --ini "$PLUGINSDIR\accounts.ini"'
    Pop $0
    Pop $1
  ${EndIf}
  nsDialogs::Create 1018
  Pop $Dialog
  ${If} $Dialog == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 0 100% 20u "$(AccIntro)"
  Pop $0
  ReadINIStr $R2 "$PLUGINSDIR\accounts.ini" "accounts" "count"
  ${If} $R2 == ""
    StrCpy $R2 0
  ${EndIf}
  StrCpy $R3 0       ; standard accounts shown
  StrCpy $R4 22      ; y position in dialog units
  StrCpy $R6 "u"
  ReadINIStr $R7 "$PLUGINSDIR\accounts.ini" "accounts" "installed"
  ; Standard accounts first (ticked), then administrators (not ticked); at most 6.
  ${For} $R8 0 1
    ${For} $R5 1 $R2
      ReadINIStr $1 "$PLUGINSDIR\accounts.ini" "a$R5" "name"
      ReadINIStr $2 "$PLUGINSDIR\accounts.ini" "a$R5" "admin"
      ${If} $2 == $R8
      ${AndIf} $R4 < 100
        ${If} $2 == 1
          StrCpy $1 "$1$(AdminSuffix)"
        ${EndIf}
        ${NSD_CreateCheckbox} 0 "$R4$R6" 100% 12u "$1"
        Pop $3
        WriteINIStr "$PLUGINSDIR\accounts.ini" "a$R5" "hwnd" $3
        ReadINIStr $4 "$PLUGINSDIR\accounts.ini" "a$R5" "protected"
        ${If} $R7 == 1
          ; Update: tick the accounts protected now.
          ${If} $4 == 1
            ${NSD_Check} $3
          ${EndIf}
        ${ElseIf} $2 == 0
          ${NSD_Check} $3
        ${EndIf}
        ${If} $2 == 0
          IntOp $R3 $R3 + 1
        ${EndIf}
        IntOp $R4 $R4 + 13
      ${EndIf}
    ${Next}
  ${Next}
  ${If} $R3 == 0
    ${NSD_CreateLabel} 0 "$R4$R6" 100% 20u "$(NoStandard)"
    Pop $0
    IntOp $R4 $R4 + 22
  ${EndIf}
  ${NSD_CreateCheckbox} 0 "$R4$R6" 100% 24u "$(Everyone)"
  Pop $EveryoneBox
  ReadINIStr $0 "$PLUGINSDIR\accounts.ini" "accounts" "mode"
  ${If} $0 == "all"
    ${NSD_Check} $EveryoneBox
  ${EndIf}
  nsDialogs::Show
FunctionEnd

Function AccountsLeave
  ${NSD_GetState} $EveryoneBox $0
  ${If} $0 == ${BST_CHECKED}
    StrCpy $Accounts "all"
    Return
  ${EndIf}
  StrCpy $Accounts ""
  ReadINIStr $R2 "$PLUGINSDIR\accounts.ini" "accounts" "count"
  ${If} $R2 == ""
    StrCpy $R2 0
  ${EndIf}
  ${For} $R5 1 $R2
    ReadINIStr $3 "$PLUGINSDIR\accounts.ini" "a$R5" "hwnd"
    ${If} $3 != ""
      ${NSD_GetState} $3 $0
      ${If} $0 == ${BST_CHECKED}
        ReadINIStr $1 "$PLUGINSDIR\accounts.ini" "a$R5" "sid"
        ${If} $Accounts == ""
          StrCpy $Accounts $1
        ${Else}
          StrCpy $Accounts "$Accounts,$1"
        ${EndIf}
      ${EndIf}
    ${EndIf}
  ${Next}
  ${If} $Accounts == ""
    MessageBox MB_OK|MB_ICONINFORMATION "$(NeedAccount)"
    Abort
  ${EndIf}
FunctionEnd

Function AgePinPage
  !insertmacro MUI_HEADER_TEXT "$(AgeTitle)" "$(AgeSub)"
  nsDialogs::Create 1018
  Pop $Dialog
  ${NSD_CreateLabel} 0 0 100% 10u "$(AgeLabel)"
  Pop $0
  ${NSD_CreateDropList} 0 12u 60% 80u ""
  Pop $BandBox
  ${NSD_CB_AddString} $BandBox "$(Band35)"
  ${NSD_CB_AddString} $BandBox "$(Band68)"
  ${NSD_CB_AddString} $BandBox "$(Band912)"
  ${NSD_CB_AddString} $BandBox "$(Band1315)"
  ${NSD_CB_AddString} $BandBox "$(Band1617)"
  ${NSD_CB_SelectString} $BandBox "$(Band912)"
  ${NSD_CreateLabel} 0 32u 100% 10u "$(PinLabel)"
  Pop $0
  ${NSD_CreatePassword} 0 44u 40% 13u ""
  Pop $Pin1Box
  ${NSD_CreateLabel} 0 62u 100% 10u "$(Pin2Label)"
  Pop $0
  ${NSD_CreatePassword} 0 74u 40% 13u ""
  Pop $Pin2Box
  ${If} $Upgrade == 1
    ${NSD_CreateLabel} 0 94u 100% 36u "$(PinHint) $(UpgradeNote)"
  ${Else}
    ${NSD_CreateLabel} 0 94u 100% 36u "$(PinHint)"
  ${EndIf}
  Pop $0
  ${NSD_SetFocus} $Pin1Box
  nsDialogs::Show
FunctionEnd

Function AgePinLeave
  ${NSD_GetText} $BandBox $0
  ${Select} $0
    ${Case} "$(Band35)"
      StrCpy $Band "3-5"
    ${Case} "$(Band68)"
      StrCpy $Band "6-8"
    ${Case} "$(Band1315)"
      StrCpy $Band "13-15"
    ${Case} "$(Band1617)"
      StrCpy $Band "16-17"
    ${CaseElse}
      StrCpy $Band "9-12"
  ${EndSelect}
  ${NSD_GetText} $Pin1Box $1
  ${NSD_GetText} $Pin2Box $2
  StrCpy $R0 $1
  Call CheckPin
  ${If} $R1 != 1
  ${OrIf} $1 != $2
    MessageBox MB_OK|MB_ICONINFORMATION "$(PinBad)"
    Abort
  ${EndIf}
  StrCpy $Pin $1
FunctionEnd

; The family DNS filter sends site names to AdGuard DNS: say so and let the
; parent switch it off (new installs; an update keeps each account's choice).
Function DnsPage
  ${If} $Upgrade == 1
    Abort
  ${EndIf}
  !insertmacro MUI_HEADER_TEXT "$(DnsTitle)" "$(DnsSub)"
  nsDialogs::Create 1018
  Pop $Dialog
  ${NSD_CreateCheckbox} 0 0 100% 12u "$(DnsCheck)"
  Pop $DnsBox
  ${If} $FamilyDns == "on"
    ${NSD_Check} $DnsBox
  ${EndIf}
  ${NSD_CreateLabel} 0 16u 100% 124u "$(DnsText)"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function DnsLeave
  ${NSD_GetState} $DnsBox $0
  ${If} $0 == ${BST_CHECKED}
    StrCpy $FamilyDns "on"
  ${Else}
    StrCpy $FamilyDns "off"
  ${EndIf}
FunctionEnd

Section "Install"
  SetRegView 64
  SetOutPath "$INSTDIR"
  ; On an update the running service holds the old program: move it aside
  ; (Windows allows renaming a running program); the agent restarts the service.
  Delete "$INSTDIR\raisesteady-guard-agent.old.exe"
  ${If} ${FileExists} "$INSTDIR\raisesteady-guard-agent.exe"
    Rename "$INSTDIR\raisesteady-guard-agent.exe" "$INSTDIR\raisesteady-guard-agent.old.exe"
  ${EndIf}
  !insertmacro PutAgent "$INSTDIR\raisesteady-guard-agent.exe"
  !if /FileExists "${DIST}\raisesteady-guard.xpi"
    File "${DIST}\raisesteady-guard.xpi"
  !endif
  File "/oname=THIRD-PARTY-NOTICES.txt" "${NOTICES}"
  !if /FileExists "${GO_NOTICE}"
    File "/oname=NOTICE-go.txt" "${GO_NOTICE}"
  !endif
  WriteUninstaller "$INSTDIR\uninstall.exe"
  ; The PIN goes to the agent in an environment variable, never on its command line.
  System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", t "$Pin")'
  DetailPrint "$(Applying)"
  nsExec::ExecToLog '"$INSTDIR\raisesteady-guard-agent.exe" install --accounts "$Accounts" --band "$Band" --family-dns "$FamilyDns" --pin-env KG_PIN'
  Pop $0
  System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", p 0)'
  StrCpy $Pin ""
  ${If} $0 == 3
    MessageBox MB_OK|MB_ICONEXCLAMATION "$(UpgradeWrongPin)" /SD IDOK
    SetErrorLevel 3
    Abort
  ${ElseIf} $0 == 4
    MessageBox MB_OK|MB_ICONEXCLAMATION "$(Locked)" /SD IDOK
    SetErrorLevel 4
    Abort
  ${ElseIf} $0 != 0
    MessageBox MB_OK|MB_ICONSTOP "$(InstallFailed) ($0)" /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "Nexodyne"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\uninstall.exe"
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
  SetShellVarContext all
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  Delete "$INSTDIR\raisesteady-guard-agent.old.exe"
  WriteINIStr "$SMPROGRAMS\${APP_NAME}\$(ShortcutName).url" "InternetShortcut" "URL" "http://127.0.0.1:${PORT}/parent?lang=$(LangCode)"
SectionEnd

; ---------------------------------------------------------------- uninstall

Function un.onInit
  SetRegView 64
  ${GetParameters} $R9
  ${GetOptions} $R9 "/PIN=" $UnPin
  ${IfNot} ${Silent}
    !insertmacro MUI_UNGETLANGUAGE
  ${EndIf}
FunctionEnd

Function un.PinPage
  !insertmacro MUI_HEADER_TEXT "$(UnTitle)" "$(UnSub)"
  nsDialogs::Create 1018
  Pop $Dialog
  ${NSD_CreateLabel} 0 0 100% 30u "$(UnIntro)"
  Pop $0
  ${NSD_CreateLabel} 0 34u 100% 10u "$(PinLabelShort)"
  Pop $0
  ${NSD_CreatePassword} 0 46u 40% 13u ""
  Pop $UnPinBox
  ${NSD_SetFocus} $UnPinBox
  nsDialogs::Show
FunctionEnd

Function un.PinLeave
  ${NSD_GetText} $UnPinBox $UnPin
  StrCpy $R0 $UnPin
  Call un.CheckPin
  ${If} $R1 != 1
    MessageBox MB_OK|MB_ICONINFORMATION "$(UnWrong)"
    Abort
  ${EndIf}
  System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", t "$UnPin")'
  nsExec::ExecToStack '"$INSTDIR\raisesteady-guard-agent.exe" verify-pin --pin-env KG_PIN'
  Pop $0
  Pop $1
  System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", p 0)'
  ${If} $0 == 4
    MessageBox MB_OK|MB_ICONINFORMATION "$(Locked)"
    Abort
  ${ElseIf} $0 != 0
    MessageBox MB_OK|MB_ICONINFORMATION "$(UnWrong)"
    Abort
  ${EndIf}
FunctionEnd

Section "Uninstall"
  SetRegView 64
  ${If} ${FileExists} "$INSTDIR\raisesteady-guard-agent.exe"
    System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", t "$UnPin")'
    DetailPrint "$(Removing)"
    nsExec::ExecToLog '"$INSTDIR\raisesteady-guard-agent.exe" uninstall --pin-env KG_PIN'
    Pop $0
    System::Call 'Kernel32::SetEnvironmentVariable(t "KG_PIN", p 0)'
    StrCpy $UnPin ""
    ${If} $0 == 3
      MessageBox MB_OK|MB_ICONINFORMATION "$(UnWrong)" /SD IDOK
      SetErrorLevel 3
      Abort
    ${ElseIf} $0 == 4
      MessageBox MB_OK|MB_ICONINFORMATION "$(Locked)" /SD IDOK
      SetErrorLevel 4
      Abort
    ${ElseIf} $0 != 0
      MessageBox MB_OK|MB_ICONSTOP "$(UnFailed) ($0)" /SD IDOK
      SetErrorLevel 1
      Abort
    ${EndIf}
  ${EndIf}
  Delete "$INSTDIR\raisesteady-guard-agent.exe"
  Delete /REBOOTOK "$INSTDIR\raisesteady-guard-agent.old.exe"
  Delete "$INSTDIR\raisesteady-guard.xpi"
  Delete "$INSTDIR\THIRD-PARTY-NOTICES.txt"
  Delete "$INSTDIR\NOTICE-go.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  SetShellVarContext all
  Delete "$SMPROGRAMS\${APP_NAME}\*.url"
  RMDir "$SMPROGRAMS\${APP_NAME}"
  DeleteRegKey HKLM "${UNINST_KEY}"
SectionEnd
