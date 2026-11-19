package zsvc

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"zpui/internal/executil"
)

// ── Автозапуск пользователя: HKCU\...\Run («как у всех приложений») ──
//
// Ведёт приложение (app_api_config.go): запись видна во вкладке «Автозапуск»
// Диспетчера задач и управляется оттуда. Служба автозапуском больше не
// занимается.

// LegacyTaskName — задача Планировщика прежней схемы автозапуска (2.0.4/2.0.5):
// миграция удаляет её и переносит состояние в Run-ключ.
const LegacyTaskName = "ZPUI"

// RestoreTaskName — внутренняя задача службы: аварийный перезапуск приложения
// после краша (запуск повышенным в сессии пользователя без UAC). Без триггера
// входа — срабатывает только по явному /RUN, поэтому в автозапуск не стреляет.
const RestoreTaskName = "ZPUI-Restore"

// StartTaskName — задача автозапуска: Run-запись запускает её через
// «schtasks /Run», а задача стартует приложение с RunLevel Highest —
// права админа при входе без запроса UAC. Без триггера входа: стреляет
// только по явному /RUN, управление — целиком у Run-записи (Диспетчер задач).
const StartTaskName = "ZPUI-Start"

// TaskXML возвращает XML-определение задачи прежней схемы (для миграции).
// ok=false, если задача не существует.
func TaskXML() (string, bool) {
	out, err := executil.HiddenCmd("schtasks", "/Query", "/TN", LegacyTaskName, "/XML").Output()
	if err != nil || !strings.Contains(string(out), "<Task") {
		return "", false
	}
	return string(out), true
}

// TaskExists — задача прежней схемы автозапуска существует.
func TaskExists() bool {
	_, ok := TaskXML()
	return ok
}

// TaskDisabled проверяет, отключена ли задача (блок Settings, Enabled=false).
func TaskDisabled() bool {
	xml, ok := TaskXML()
	if !ok {
		return false
	}
	start := strings.Index(xml, "<Settings>")
	if start < 0 {
		return false
	}
	rest := xml[start:]
	end := strings.Index(rest, "</Settings>")
	if end < 0 {
		return false
	}
	return strings.Contains(rest[:end], "<Enabled>false</Enabled>")
}

// TaskEffectiveEnabled — реальное состояние прежней задачи: существует и не отключена.
func TaskEffectiveEnabled() bool {
	if !TaskExists() {
		return false
	}
	return !TaskDisabled()
}

// RemoveLegacyTask удаляет задачу прежней схемы автозапуска.
func RemoveLegacyTask() {
	executil.HiddenCmd("schtasks", "/Delete", "/TN", LegacyTaskName, "/f").Run()
}

// CreateRestoreTask создаёт внутреннюю задачу аварийного перезапуска: триггер
// ONCE на далёкое будущее (сама по себе не срабатывает), /RUN — по команде
// watchdog'а. RunLevel Highest — запуск приложения повышенным без UAC;
// без лимита времени и с разрешением работы от батареи.
func CreateRestoreTask(exePath string, hidden bool) error {
	return createRunLevelTask(RestoreTaskName, exePath, hidden)
}

// CreateStartTask создаёт задачу автозапуска (см. StartTaskName): та же
// механика, что у задачи восстановления, но её /RUN делает Run-запись при входе.
func CreateStartTask(exePath string, hidden bool) error {
	return createRunLevelTask(StartTaskName, exePath, hidden)
}

// createRunLevelTask регистрирует задачу без триггера входа с RunLevel Highest.
// Register-ScheduledTask, а не schtasks: у schtasks-задач запрет запуска от
// батареи и кавычки внутри <Command> при пути с пробелами.
func createRunLevelTask(name, exePath string, hidden bool) error {
	script := fmt.Sprintf("$a = New-ScheduledTaskAction -Execute '%s'", strings.ReplaceAll(exePath, "'", "''"))
	if hidden {
		script += " -Argument '--hidden'"
	}
	script += fmt.Sprintf("; $t = New-ScheduledTaskTrigger -Once -At (Get-Date -Year 2099 -Month 1 -Day 1); "+
		"$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries "+
		"-ExecutionTimeLimit (New-TimeSpan -Seconds 0); "+
		"Register-ScheduledTask -TaskName '%s' -Action $a -Trigger $t -Settings $s -RunLevel Highest -Force | Out-Null", name)
	out, err := executil.HiddenCmd("powershell", "-NoProfile", "-EncodedCommand",
		base64.StdEncoding.EncodeToString(utf16LE(script))).CombinedOutput()
	if err != nil {
		return fmt.Errorf("register-scheduled-task: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveStartTask удаляет задачу автозапуска.
func RemoveStartTask() {
	executil.HiddenCmd("schtasks", "/Delete", "/TN", StartTaskName, "/f").Run()
}

// RestoreTaskExists — внутренняя задача перезапуска существует.
func RestoreTaskExists() bool {
	out, err := executil.HiddenCmd("schtasks", "/Query", "/TN", RestoreTaskName).Output()
	return err == nil && strings.Contains(string(out), RestoreTaskName)
}

// RunRestoreTask запускает задачу аварийного перезапуска сейчас.
func RunRestoreTask() error {
	out, err := executil.HiddenCmd("schtasks", "/Run", "/TN", RestoreTaskName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks /Run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return b
}
