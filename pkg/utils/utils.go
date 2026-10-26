package utils

import (
	"crypto/md5"
	"encoding/hex"
)

// GenerateHash generates a hash which is used as suffix in veth in for both end.
// To identify the veth pair in host and container
func GenerateHash(containerID string) string {
	hash := md5.Sum([]byte(containerID))
	return hex.EncodeToString(hash[:])
}
