package dispatcher

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

func asinfo(command string) (string, error) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:3000", 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte(command)
	header := (uint64(2) << 56) | (uint64(1) << 48) | uint64(len(payload))
	var h [8]byte
	binary.BigEndian.PutUint64(h[:], header)
	if _, err := conn.Write(append(h[:], payload...)); err != nil {
		return "", err
	}
	if _, err := io.ReadFull(conn, h[:]); err != nil {
		return "", err
	}
	size := binary.BigEndian.Uint64(h[:]) & 0x0000ffffffffffff
	if size == 0 || size > 1024*1024 {
		return "", fmt.Errorf("invalid asinfo response size %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf)), nil
}
