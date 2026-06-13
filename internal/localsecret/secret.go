package localsecret

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Ensure(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		secret := strings.TrimSpace(string(data))
		if secret == "" {
			return "", fmt.Errorf("local secret file is empty: %s", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", err
		}
		return secret, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(randomBytes)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return Ensure(path)
		}
		return "", err
	}
	if _, err := file.WriteString(secret + "\n"); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return secret, nil
}
