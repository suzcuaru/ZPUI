package notify

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32                            = windows.NewLazySystemDLL("shell32.dll")
	modOle32                              = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx                    = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize                    = modOle32.NewProc("CoUninitialize")
	procSHGetPropertyStoreFromParsingName = modShell32.NewProc("SHGetPropertyStoreFromParsingName")

	iidIPropertyStore = windows.GUID{
		Data1: 0x886D8EEB, Data2: 0x8CF2, Data3: 0x4446,
		Data4: [8]byte{0x8D, 0x02, 0xCD, 0xBA, 0x1D, 0xBD, 0xCF, 0x99},
	}

	pkeyAppUserModelID = propertyKey{
		fmtid: windows.GUID{
			Data1: 0x9F4C2855, Data2: 0x9F79, Data3: 0x4B39,
			Data4: [8]byte{0xA8, 0xD0, 0xE1, 0xD4, 0x2D, 0xE1, 0xD5, 0xF3},
		},
		pid: 5,
	}
)

const vtLPWSTR uint16 = 31

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

type propVariant struct {
	vt          uint16
	r1, r2, r3  uint16
	pwszVal     uintptr
}

// ensureStartMenuShortcut создаёт ярлык .lnk в Start Menu и устанавливает
// на нём свойство AppUserModelID. Без этого WinRT Toast silently drop'ает
// уведомления для Win32-приложений. Должна вызываться на COM-инициализированном потоке.
func ensureStartMenuShortcut(exePath, aumid string) error {
	programsDir := filepath.Join(os.Getenv("APPDATA"),
		"Microsoft", "Windows", "Start Menu", "Programs")
	shortcutPath := filepath.Join(programsDir, "ZPUI.lnk")

	if _, err := os.Stat(shortcutPath); err == nil {
		return nil
	}

	if err := createShortcutLNK(shortcutPath, exePath); err != nil {
		return err
	}

	return setShortcutAumid(shortcutPath, aumid)
}

func createShortcutLNK(shortcutPath, targetPath string) error {
	script := "$s=(New-Object -COM WScript.Shell).CreateShortcut('" + shortcutPath +
		"'); $s.TargetPath='" + targetPath +
		"'; $s.WorkingDirectory='" + filepath.Dir(targetPath) +
		"'; $s.Save()"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func setShortcutAumid(shortcutPath, aumid string) error {
	procCoInitializeEx.Call(0, 2)
	defer procCoUninitialize.Call()

	var store unsafe.Pointer
	shortcutPtr, _ := windows.UTF16PtrFromString(shortcutPath)
	hr, _, _ := procSHGetPropertyStoreFromParsingName.Call(
		uintptr(unsafe.Pointer(shortcutPtr)),
		0, 0,
		uintptr(unsafe.Pointer(&iidIPropertyStore)),
		uintptr(unsafe.Pointer(&store)),
	)
	if hr != 0 || store == nil {
		return oleErr("SHGetPropertyStoreFromParsingName", hr)
	}
	defer comRelease(store)

	wStr, _ := windows.UTF16PtrFromString(aumid)
	pv := propVariant{vt: vtLPWSTR, pwszVal: uintptr(unsafe.Pointer(wStr))}

	r1 := comSetValue(store,
		uintptr(unsafe.Pointer(&pkeyAppUserModelID)),
		uintptr(unsafe.Pointer(&pv)),
	)
	if r1 != 0 {
		return oleErr("SetValue", r1)
	}

	comCommit(store)
	return nil
}

func vtableFunc(obj unsafe.Pointer, idx int) uintptr {
	vtable := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(vtable, idx*int(unsafe.Sizeof(uintptr(0)))))
}

func comRelease(obj unsafe.Pointer) {
	syscall.SyscallN(vtableFunc(obj, 2), uintptr(obj))
}

func comSetValue(obj unsafe.Pointer, key, val uintptr) uintptr {
	r1, _, _ := syscall.SyscallN(vtableFunc(obj, 6), uintptr(obj), key, val)
	return r1
}

func comCommit(obj unsafe.Pointer) {
	syscall.SyscallN(vtableFunc(obj, 7), uintptr(obj))
}

func oleErr(ctx string, hr uintptr) error {
	return &notifyError{ctx: ctx, hr: hr}
}

type notifyError struct {
	ctx string
	hr  uintptr
}

func (e *notifyError) Error() string {
	return e.ctx + " HRESULT=0x" + uintptrHex(e.hr)
}

func uintptrHex(v uintptr) string {
	const hex = "0123456789ABCDEF"
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 16)
	for v > 0 {
		buf = append([]byte{hex[v&0xF]}, buf...)
		v >>= 4
	}
	return string(buf)
}
