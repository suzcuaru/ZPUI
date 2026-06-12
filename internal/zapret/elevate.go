package zapret

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func elevatedSCRemove() error {
	resultFile := filepath.Join(os.TempDir(), "zpui_svc_result.txt")
	bat := fmt.Sprintf(`@echo off
net stop zapret >nul 2>&1
sc delete zapret >nul 2>&1
taskkill /IM winws.exe /F >nul 2>&1
net stop WinDivert >nul 2>&1
sc delete WinDivert >nul 2>&1
net stop WinDivert14 >nul 2>&1
sc delete WinDivert14 >nul 2>&1
echo REMOVED > "%s"
`, resultFile)

	tmpBat := filepath.Join(os.TempDir(), "zpui_svc_remove.bat")
	if err := os.WriteFile(tmpBat, []byte(bat), 0644); err != nil {
		return fmt.Errorf("write temp bat: %w", err)
	}
	defer os.Remove(tmpBat)

	os.Remove(resultFile)

	psCmd := fmt.Sprintf(`Start-Process cmd.exe -ArgumentList '/c "%s"' -Verb RunAs -Wait`, tmpBat)
	psOut, psErr := exec.Command("powershell", "-NoProfile", "-Command", psCmd).CombinedOutput()

	resultData, _ := os.ReadFile(resultFile)
	os.Remove(resultFile)
	result := string(resultData)

	if psErr != nil {
		return fmt.Errorf("elevation failed: %v: %s: %s", psErr, string(psOut), result)
	}

	if result == "REMOVED" {
		return nil
	}
	return fmt.Errorf("service remove failed or cancelled (result: %q)", result)
}

func elevatedSCCreate(exePath, args, strategyName string) error {
	resultFile := filepath.Join(os.TempDir(), "zpui_svc_result.txt")
	bat := fmt.Sprintf(`@echo off
sc create zapret binPath= "\"%s\" %s" DisplayName= zapret start= auto
if %%errorlevel%% neq 0 (
    echo SC_CREATE_FAILED > "%s"
    exit /b 1
)
sc description zapret "Zapret DPI bypass software" >nul 2>&1
reg add "HKLM\System\CurrentControlSet\Services\zapret" /v zapret-discord-youtube /t REG_SZ /d "%s" /f >nul 2>&1
sc start zapret
if %%errorlevel%% neq 0 (
    echo SC_START_FAILED > "%s"
    exit /b 1
)
echo SERVICE_OK > "%s"
`, exePath, args, resultFile, strategyName, resultFile, resultFile)

	tmpBat := filepath.Join(os.TempDir(), "zpui_svc_install.bat")
	if err := os.WriteFile(tmpBat, []byte(bat), 0644); err != nil {
		return fmt.Errorf("write temp bat: %w", err)
	}
	defer os.Remove(tmpBat)

	os.Remove(resultFile)

	psCmd := fmt.Sprintf(`Start-Process cmd.exe -ArgumentList '/c "%s"' -Verb RunAs -Wait`, tmpBat)
	psOut, psErr := exec.Command("powershell", "-NoProfile", "-Command", psCmd).CombinedOutput()

	resultData, _ := os.ReadFile(resultFile)
	os.Remove(resultFile)
	result := string(resultData)

	if psErr != nil {
		return fmt.Errorf("elevation failed: %v: %s: %s", psErr, string(psOut), result)
	}

	switch {
	case result == "SERVICE_OK":
		return nil
	case result == "SC_CREATE_FAILED":
		return fmt.Errorf("sc create failed (binPath or args invalid)")
	case result == "SC_START_FAILED":
		return fmt.Errorf("service created but sc start failed")
	case len(result) == 0:
		return fmt.Errorf("elevation completed but no result file — user may have cancelled UAC or bat exited early")
	default:
		return fmt.Errorf("unexpected result: %s", result)
	}
}
