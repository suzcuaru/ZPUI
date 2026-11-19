// Package winopt — игровые твики Windows, снижающие задержку TCP:
// TcpAckFrequency=1 (без отложенных ACK) и TCPNoDelay=1 (без Nagle)
// на каждом интерфейсе + NetworkThrottlingIndex=0xFFFFFFFF (без сетевого
// троттлинга мультимедиа). UDP-игры трвики не задевают. Единственный
// источник истины — реестр, как с автозапуском: включённость читается
// оттуда же, где применяется. Нужны права администратора.
package winopt

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

const (
	tcpipParamsKey = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`
	interfacesKey  = tcpipParamsKey + `\Interfaces`
	throttlingOff  = 0xFFFFFFFF
)

// Status — состояние твиков в реестре.
type Status struct {
	Applied    bool `json:"applied"`
	Interfaces int  `json:"interfaces"`
	WithTweaks int  `json:"with_tweaks"`
	Throttling bool `json:"throttling_off"`
}

// Read читает фактическое состояние твиков.
func Read() (Status, error) {
	st := Status{}
	ifaces, err := interfaceNames()
	if err != nil {
		return st, err
	}
	st.Interfaces = len(ifaces)
	for _, name := range ifaces {
		ok, err := interfaceTweaked(name)
		if err != nil {
			return st, err
		}
		if ok {
			st.WithTweaks++
		}
	}
	st.Throttling = throttlingDisabled()
	st.Applied = st.Interfaces > 0 && st.WithTweaks == st.Interfaces && st.Throttling
	return st, nil
}

// Apply включает твики на всех текущих интерфейсах. Повторный вызов
// идемпотентен; на новых интерфейсах (смена сети) нужно применить снова.
func Apply() (Status, error) {
	ifaces, err := interfaceNames()
	if err != nil {
		return Status{}, err
	}
	if len(ifaces) == 0 {
		return Status{}, fmt.Errorf("winopt: сетевые интерфейсы не найдены")
	}
	for _, name := range ifaces {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, interfacesKey+`\`+name, registry.SET_VALUE)
		if err != nil {
			return Status{}, fmt.Errorf("winopt: открыть интерфейс %s: %w", name, err)
		}
		if err := k.SetDWordValue("TcpAckFrequency", 1); err != nil {
			k.Close()
			return Status{}, fmt.Errorf("winopt: TcpAckFrequency %s: %w", name, err)
		}
		if err := k.SetDWordValue("TCPNoDelay", 1); err != nil {
			k.Close()
			return Status{}, fmt.Errorf("winopt: TCPNoDelay %s: %w", name, err)
		}
		k.Close()
	}
	if err := setThrottling(throttlingOff); err != nil {
		return Status{}, err
	}
	return Read()
}

// Remove снимает твики: значения удаляются, система возвращается к
// поведению по умолчанию (отложенные ACK, Nagle, троттлинг включён).
func Remove() (Status, error) {
	ifaces, err := interfaceNames()
	if err != nil {
		return Status{}, err
	}
	var firstErr error
	for _, name := range ifaces {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, interfacesKey+`\`+name, registry.SET_VALUE)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("winopt: открыть интерфейс %s: %w", name, err)
			}
			continue
		}
		_ = k.DeleteValue("TcpAckFrequency")
		_ = k.DeleteValue("TCPNoDelay")
		k.Close()
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpipParamsKey, registry.SET_VALUE)
	if err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("winopt: открыть Tcpip\\Parameters: %w", err)
		}
	} else {
		_ = k.DeleteValue("NetworkThrottlingIndex")
		k.Close()
	}
	if firstErr != nil {
		return Status{}, firstErr
	}
	return Read()
}

func interfaceNames() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, interfacesKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("winopt: открыть Interfaces: %w", err)
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("winopt: список интерфейсов: %w", err)
	}
	return names, nil
}

func interfaceTweaked(name string) (bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, interfacesKey+`\`+name, registry.QUERY_VALUE)
	if err != nil {
		return false, nil
	}
	defer k.Close()
	ack, _, err := k.GetIntegerValue("TcpAckFrequency")
	if err != nil || ack != 1 {
		return false, nil
	}
	nodelay, _, err := k.GetIntegerValue("TCPNoDelay")
	if err != nil || nodelay != 1 {
		return false, nil
	}
	return true, nil
}

func throttlingDisabled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpipParamsKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("NetworkThrottlingIndex")
	return err == nil && v == throttlingOff
}

func setThrottling(v uint32) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, tcpipParamsKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("winopt: открыть Tcpip\\Parameters: %w", err)
	}
	defer k.Close()
	if err := k.SetDWordValue("NetworkThrottlingIndex", v); err != nil {
		return fmt.Errorf("winopt: NetworkThrottlingIndex: %w", err)
	}
	return nil
}
