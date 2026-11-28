package vault

// RestoreBackup validates ciphertext and the password in an isolated staging
// directory, then installs it into a new/empty directory.
//
// Test-only wrapper: production callers use RestoreBackupForVault with the
// bound vault identity.
func RestoreBackup(archive []byte, destination, password string) error {
	return RestoreBackupForVault(archive, destination, password, "")
}
