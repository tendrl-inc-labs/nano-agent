//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	bolt "go.etcd.io/bbolt"
)

const (
	ACL_REVISION              = 2
	SE_FILE_OBJECT            = 1
	DACL_SECURITY_INFORMATION = 0x4
)

// Dynamically load Windows DLLs and functions
var (
	advapi32                  = syscall.NewLazyDLL("advapi32.dll")
	procInitializeAcl         = advapi32.NewProc("InitializeAcl")
	procAddAccessAllowedAce   = advapi32.NewProc("AddAccessAllowedAce")
	procSetNamedSecurityInfoW = advapi32.NewProc("SetNamedSecurityInfoW")
)

// MEMORYSTATUSEX represents the memory status structure used by GlobalMemoryStatusEx
type MEMORYSTATUSEX struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// createGroup creates a local group (e.g., "tendrl") if it doesn't already exist
func createGroup(groupName string) error {
	cmd := exec.Command("net", "localgroup", groupName)
	if err := cmd.Run(); err == nil {
		fmt.Printf("Group '%s' already exists\n", groupName)
		return nil
	}

	cmd = exec.Command("net", "localgroup", groupName, "/add")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create group '%s': %w", groupName, err)
	}

	fmt.Printf("Group '%s' created successfully\n", groupName)
	return nil
}

// getGroupSID retrieves the SID of the specified group
func getGroupSID(groupName string) (*syscall.SID, error) {
	groupNameUTF16, err := syscall.UTF16PtrFromString(groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to convert group name to UTF-16: %w", err)
	}

	var sidSize uint32
	var domainSize uint32
	var sidUse uint32

	err = syscall.LookupAccountName(nil, groupNameUTF16, nil, &sidSize, nil, &domainSize, &sidUse)
	if err != syscall.ERROR_INSUFFICIENT_BUFFER {
		return nil, fmt.Errorf("failed to get SID buffer size: %w", err)
	}

	sid := make([]byte, sidSize)
	domain := make([]uint16, domainSize)

	err = syscall.LookupAccountName(nil, groupNameUTF16, (*syscall.SID)(unsafe.Pointer(&sid[0])), &sidSize, &domain[0], &domainSize, &sidUse)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve SID for group '%s': %w", groupName, err)
	}

	return (*syscall.SID)(unsafe.Pointer(&sid[0])), nil
}

// setWindowsACL assigns the specified group permissions on the directory
func setWindowsACL(dirPath, groupName string) error {
	groupSID, err := getGroupSID(groupName)
	if err != nil {
		return fmt.Errorf("failed to get SID for group '%s': %w", groupName, err)
	}

	// Initialize an ACL
	acl := make([]byte, 1024)
	ret, _, err := procInitializeAcl.Call(uintptr(unsafe.Pointer(&acl[0])), uintptr(len(acl)), uintptr(ACL_REVISION))
	if ret == 0 {
		return fmt.Errorf("failed to initialize ACL: %v", err)
	}

	// Add an ACE (Access Control Entry) for the group SID
	ret, _, err = procAddAccessAllowedAce.Call(uintptr(unsafe.Pointer(&acl[0])), uintptr(ACL_REVISION), syscall.GENERIC_ALL, uintptr(unsafe.Pointer(groupSID)))
	if ret == 0 {
		return fmt.Errorf("failed to add ACE to ACL: %v", err)
	}

	// Apply the ACL to the directory
	dirPathUTF16, err := syscall.UTF16PtrFromString(dirPath)
	if err != nil {
		return fmt.Errorf("failed to convert directory path to UTF-16: %w", err)
	}

	ret, _, err = procSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(dirPathUTF16)),
		uintptr(SE_FILE_OBJECT),
		uintptr(DACL_SECURITY_INFORMATION),
		0, 0, uintptr(unsafe.Pointer(&acl[0])), 0,
	)
	if ret != 0 {
		return fmt.Errorf("failed to set security info: %v", err)
	}

	return nil
}

// Free calculates free memory and disk usage on Windows
func Free() map[string]interface{} {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	memStatus := MEMORYSTATUSEX{
		Length: uint32(unsafe.Sizeof(MEMORYSTATUSEX{})),
	}

	// Call the GlobalMemoryStatusEx function
	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return map[string]interface{}{
			"error": fmt.Sprintf("failed to get memory stats: %v", err),
		}
	}

	// Convert memory stats to human-readable format
	availPhysHuman, availUnit := Convert(memStatus.AvailPhys)
	totalPhysHuman, totalUnit := Convert(memStatus.TotalPhys)

	return map[string]interface{}{
		"mem_free":  fmt.Sprintf("%.2f %s", availPhysHuman, availUnit),
		"mem_total": fmt.Sprintf("%.2f %s", totalPhysHuman, totalUnit),
	}
}

// CreateDirs sets up the environment, including directory creation and permissions
func CreateDirs(dirPath string) error {
	groupName := "tendrl"

	if err := createGroup(groupName); err != nil {
		return err
	}

	if err := os.MkdirAll(dirPath, 0750); err != nil {
		return fmt.Errorf("failed to create directory '%s': %w", dirPath, err)
	}

	if err := setWindowsACL(dirPath, groupName); err != nil {
		return fmt.Errorf("failed to set ACL: %w", err)
	}

	db, err := bolt.Open(fmt.Sprintf("%s\\tether.db", dirPath), 0660, nil)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Directory setup complete: %s\n", dirPath)
	return nil
}
