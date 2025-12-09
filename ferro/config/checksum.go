package config

import (
	"fmt"
	"hash/crc32"
)

type Checksum string

func CalculateChecksum(raw []byte) Checksum {
	return Checksum(fmt.Sprintf("%08x", crc32.ChecksumIEEE(raw)))
}

