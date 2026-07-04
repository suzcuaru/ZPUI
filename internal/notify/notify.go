package notify

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func Show(title, body string) error {
	t := strings.ReplaceAll(title, "'", "''")
	b := strings.ReplaceAll(body, "'", "''")

	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
Try{
[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]|Out-Null
$x=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$x.GetElementsByTagName('text').Item(0).AppendChild($x.CreateTextNode('%s'))|Out-Null
$x.GetElementsByTagName('text').Item(1).AppendChild($x.CreateTextNode('%s'))|Out-Null
$t=[Windows.UI.Notifications.ToastNotification]::new($x)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('ZPUI').Show($t)
}Catch{
Add-Type -AssemblyName System.Windows.Forms
$n=New-Object System.Windows.Forms.NotifyIcon
$n.Icon=[System.Drawing.SystemIcons]::Information
$n.Visible=$true
$n.ShowBalloonTip(5000,'%s','%s',[System.Windows.Forms.ToolTipIcon]::Info)
Start-Sleep -Milliseconds 5500
$n.Dispose()
}`, t, b, t, b)

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-NoProfile", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
