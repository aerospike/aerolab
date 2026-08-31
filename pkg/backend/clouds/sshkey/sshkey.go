// Package sshkey manages the SSH key pair that a project's instances are
// created with.
package sshkey

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/citrusleaf/aerolab/pkg/utils/file"
	"github.com/rglonek/logger"
	"golang.org/x/crypto/ssh"
)

// keyBits is the size of the generated RSA key.
const keyBits = 2048

// ensureLock serialises the goroutines of one process. Key creation is rare
// and takes tens of milliseconds, so a single lock for every project costs
// nothing worth splitting it up for.
var ensureLock sync.Mutex

// Ensure returns the public half of the key pair called name under dir,
// generating and storing the pair when it is not on disk yet.
//
// Instance creation races, both between the goroutines of one process and
// between aerolab processes that share a root directory, and exactly one pair
// may win: an instance is created trusting the public key its caller was
// handed, so it is unreachable forever if a different private key ends up on
// disk. The check and the create therefore run under a mutex and a lock file,
// and each file is replaced atomically so that a reader elsewhere never picks
// up half a key.
func Ensure(dir string, name string, log *logger.Logger) ([]byte, error) {
	ensureLock.Lock()
	defer ensureLock.Unlock()

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create ssh keys directory %s: %w", dir, err)
	}
	lock, err := file.AcquireLock(filepath.Join(dir, name+".lock"))
	if err != nil {
		return nil, err
	}
	defer lock.Release() //nolint:errcheck

	privatePath := filepath.Join(dir, name)
	publicPath := privatePath + ".pub"
	publicKey, err := load(privatePath, publicPath, log)
	if err != nil {
		return nil, err
	}
	if publicKey != nil {
		return publicKey, nil
	}
	return create(privatePath, publicPath, log)
}

// load returns the stored public key, or nil when the pair has to be created.
// A pair that is only half there is discarded rather than completed: the two
// files have to match, and there is no way to recover one from the other.
func load(privatePath string, publicPath string, log *logger.Logger) ([]byte, error) {
	privateExists, err := exists(privatePath)
	if err != nil {
		return nil, err
	}
	publicExists, err := exists(publicPath)
	if err != nil {
		return nil, err
	}
	switch {
	case !privateExists && !publicExists:
		log.Detail("SSH key %s does not exist, creating it", privatePath)
		return nil, nil
	case !privateExists || !publicExists:
		log.Detail("SSH key pair is incomplete (private: %v, public: %v), recreating it", privateExists, publicExists)
		os.Remove(privatePath)
		os.Remove(publicPath)
		return nil, nil
	}
	publicKey, err := os.ReadFile(publicPath)
	if err != nil {
		log.Detail("SSH public key %s cannot be read (%s), recreating the key pair", publicPath, err)
		os.Remove(privatePath)
		os.Remove(publicPath)
		return nil, nil
	}
	log.Detail("SSH key pair found at %s and %s", privatePath, publicPath)
	return trim(publicKey), nil
}

func create(privatePath string, publicPath string, log *logger.Logger) ([]byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create public key: %w", err)
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)
	privateKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := file.Store(privatePath, ".tmp", 0600, privateKeyBytes); err != nil {
		return nil, fmt.Errorf("failed to save private key: %w", err)
	}
	if err := file.Store(publicPath, ".tmp", 0600, publicKeyBytes); err != nil {
		return nil, fmt.Errorf("failed to save public key: %w", err)
	}
	log.Detail("SSH key pair created at %s and %s", privatePath, publicPath)
	return trim(publicKeyBytes), nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to stat %s: %w", path, err)
}

func trim(publicKey []byte) []byte {
	return bytes.Trim(publicKey, "\n\r\t ")
}
