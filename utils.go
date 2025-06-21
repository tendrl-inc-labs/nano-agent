//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	bolt "go.etcd.io/bbolt"
)

// createGroup creates a local group (e.g., "tendrl") if it doesn't already exist
func createGroup(groupName string) error {
	cmd := exec.Command("getent", "group", groupName)
	if err := cmd.Run(); err == nil {
		// Group exists
		fmt.Printf("Group '%s' already exists\n", groupName)
		return nil
	}

	cmd = exec.Command("sudo", "groupadd", groupName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create group '%s': %w", groupName, err)
	}

	fmt.Printf("Group '%s' created successfully\n", groupName)
	return nil
}

// setPermissions sets the group ownership and permissions for a directory
func setPermissions(dirPath, groupName string) error {
	cmd := exec.Command("sudo", "chown", fmt.Sprintf(":%s", groupName), dirPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set group ownership for '%s': %w", dirPath, err)
	}

	if err := os.Chmod(dirPath, 0770); err != nil {
		return fmt.Errorf("failed to set permissions for '%s': %w", dirPath, err)
	}

	return nil
}

// CreateDirs sets up the environment, including directory creation and permissions
func CreateDirs(dirPath string) error {
	groupName := "tendrl"

	// Create the group if it doesn't exist
	if err := createGroup(groupName); err != nil {
		return err
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0750); err != nil {
		return fmt.Errorf("failed to create directory '%s': %w", dirPath, err)
	}

	// Set permissions for the directory
	if err := setPermissions(dirPath, groupName); err != nil {
		return err
	}

	// Initialize the database in the directory
	db, err := bolt.Open(fmt.Sprintf("%s/tether.db", dirPath), 0660, nil)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	fmt.Printf("Directory setup complete: %s\n", dirPath)
	return nil
}

// Free calculates free memory and disk usage
func Free() map[string]interface{} {
	var stat syscall.Statfs_t
	err := syscall.Statfs("/", &stat)
	if err != nil {
		return map[string]interface{}{
			"error": "failed to get disk stats",
		}
	}

	fsSize := stat.Blocks * uint64(stat.Bsize)
	fsFree := stat.Bfree * uint64(stat.Bsize)
	fsFreeHuman, fsFreeUnit := Convert(fsFree)
	fsSizeHuman, fsSizeUnit := Convert(fsSize)

	return map[string]interface{}{
		"disk_free": fmt.Sprintf("%.2f %s", fsFreeHuman, fsFreeUnit),
		"disk_size": fmt.Sprintf("%.2f %s", fsSizeHuman, fsSizeUnit),
	}
}
