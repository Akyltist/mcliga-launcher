//go:build gui

package main

// Окно выбора папки средствами Windows.

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	ole32              = syscall.NewLazyDLL("ole32.dll")
	procBrowse         = shell32.NewProc("SHBrowseForFolderW")
	procPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoInit         = ole32.NewProc("CoInitializeEx")
	procCoUninit       = ole32.NewProc("CoUninitialize")
	procCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Fn          uintptr
	LParam      uintptr
	Image       int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040
)

// pickFolder показывает системное окно выбора папки. Пустая строка: пользователь отказался.
func pickFolder(owner uintptr, title string) string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procCoInit.Call(0, 2) // COINIT_APARTMENTTHREADED
	defer procCoUninit.Call()

	name := make([]uint16, 260)
	t, _ := syscall.UTF16PtrFromString(title)
	bi := browseInfo{Owner: owner, DisplayName: &name[0], Title: t, Flags: bifReturnOnlyFSDirs | bifNewDialogStyle}
	pidl, _, _ := procBrowse.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl)
	buf := make([]uint16, 520)
	ok, _, _ := procPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
