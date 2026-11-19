package app

// Внутренние перечисления процессов и адаптеров вместо PowerShell.
// PS-дети подвешивали стартовый экран на холодной машине (зависали без
// таймаута, баг 06.10), владелец попросил уйти от PowerShell совсем —
// здесь чистые вызовы Windows: Toolhelp32Snapshot для процессов и
// GetAdaptersAddresses (iphlpapi) для адаптеров.

import (
	"errors"
	"strings"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// oemToUTF8 — декодирует вывод консольных утилит (sc, rasdial) из системной
// OEM-кодировки (у русского Windows это cp866): без PowerShell их байты
// приходят в OEM, и без декода русские имена/фразы превращаются в кракозябры,
// а маркеры вроде «Нет подключений» перестают находиться.
var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")

func oemToUTF8(s string) string {
	if utf8.ValidString(s) {
		return s // уже UTF-8 (бывает на части конфигураций) — не портим
	}
	r1, _, _ := procGetOEMCP.Call()
	var enc encoding.Encoding
	switch uint32(r1) {
	case 866:
		enc = charmap.CodePage866
	case 437:
		enc = charmap.CodePage437
	case 850:
		enc = charmap.CodePage850
	case 852:
		enc = charmap.CodePage852
	default:
		return s // незнакомая кодировка — отдаём как есть
	}
	out, err := enc.NewDecoder().Bytes([]byte(s))
	if err != nil {
		return s
	}
	return string(out)
}

type procEntry struct {
	name string // lowercase, с расширением
	path string // полный путь образа; пусто — не удалось открыть
}

var (
	modkernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procQueryFullProcessImageName = modkernel32.NewProc("QueryFullProcessImageNameW")
)

// processSnapshot — все процессы системы. Имя берётся из самой записи
// снимка; путь образа — QueryFullProcessImageNameW, для которого хватает
// PROCESS_QUERY_LIMITED_INFORMATION (работает и на elevated-процессах).
func processSnapshot() []procEntry {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return nil
	}
	out := make([]procEntry, 0, 128)
	for {
		e := procEntry{name: strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))}
		if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID); err == nil {
			var buf [1024]uint16
			n := uint32(len(buf))
			if r1, _, callErr := procQueryFullProcessImageName.Call(
				uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r1 != 0 && n > 0 {
				e.path = strings.ToLower(windows.UTF16ToString(buf[:n]))
			} else {
				_ = callErr
			}
			_ = windows.CloseHandle(h)
		}
		out = append(out, e)
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return out
}

// adapterPair — имя и описание сетевого адаптера.
type adapterPair struct {
	Name string
	Desc string
}

// upAdapters — активные сетевые адаптеры (GetAdaptersAddresses, AF_UNSPEC).
func upAdapters() []adapterPair {
	size := uint32(16 << 10)
	var p *windows.IpAdapterAddresses
	var buf []byte
	for i := 0; i < 4; i++ {
		buf = make([]byte, size)
		p = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, p, &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil
		}
	}
	var out []adapterPair
	for ; p != nil; p = p.Next {
		if p.OperStatus != windows.IfOperStatusUp {
			continue
		}
		out = append(out, adapterPair{
			Name: windows.UTF16PtrToString(p.FriendlyName),
			Desc: windows.UTF16PtrToString(p.Description),
		})
	}
	return out
}
