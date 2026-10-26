package flyout

// Topmost keeps a normal window above other windows without taking focus.
func Topmost(hwnd uintptr) {
	pSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, swpNoSize|swpNoMove|swpNoActivate)
}
