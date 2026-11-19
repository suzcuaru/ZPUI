package app

import (
	"golang.org/x/sys/windows/registry"
	"zpui/internal/database"
	"zpui/internal/executil"
)

// HVCI (Целостность памяти) блокирует загрузку WinDivert64.sys — драйвер
// числится в списке уязвимых драйверов Microsoft. Точечного исключения для
// драйвера не существует (блоклист фиксированный, обновляется Microsoft),
// единственный путь — отключить политику и перезагрузить ПК.

const hvciKeyPath = `SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity`

// hvciEnabled — включена ли «Целостность памяти» (HVCI). С включённой HVCI
// Windows блокирует загрузку WinDivert-драйвера (он числится в списке
// уязвимых драйверов Microsoft) — движок zapret умирает сразу после старта,
// а все фазы восстановления падают с R-000.
func hvciEnabled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, hvciKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("Enabled")
	return err == nil && v == 1
}

// GetHvciFix — состояние политики для экрана ошибки.
func (a *App) GetHvciFix() map[string]interface{} {
	return okRespWithData(map[string]interface{}{
		"enabled": hvciEnabled(),
	})
}

// FixHvci отключает Целостность памяти (Enabled=0) и инициирует
// перезагрузку: без ребута драйвер не разблокируется. Если политика
// закреплена UEFI-замком или групповой политикой — запись не пройдёт,
// возвращаем ошибку с ручным путём.
func (a *App) FixHvci() map[string]interface{} {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, hvciKeyPath, registry.SET_VALUE)
	if err != nil {
		return errResp("ключ политики недоступен (возможен UEFI-замок или политика организации): " + err.Error())
	}
	if err := k.SetDWordValue("Enabled", 0); err != nil {
		k.Close()
		return errResp("не удалось записать политику: " + err.Error())
	}
	k.Close()

	a.log.Warn("hvci", "Целостность памяти отключена по действию пользователя — перезагрузка")
	database.InsertActionLog(&database.ActionLog{
		Category: "system",
		Action:   "hvci_disable",
		Details:  "Отключена Целостность памяти (HVCI) для разблокировки WinDivert — перезагрузка",
	})
	executil.HiddenCmd("shutdown", "/r", "/t", "8", "/c", "ZPUI: отключение Целостности памяти — ПК перезагрузится через 8 секунд")
	return okRespWithData(map[string]interface{}{"rebooting": true})
}
