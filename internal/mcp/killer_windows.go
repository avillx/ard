//go:build windows

package mcp

// func killProcessGroup(cmd *exec.Cmd) error { //nolint: unused // keep it
// 	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() //nolint: gosec,noctx
// }
//
// func setProcAttrs(cmd *exec.Cmd) { //nolint: unused // keep it
// 	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
// }
