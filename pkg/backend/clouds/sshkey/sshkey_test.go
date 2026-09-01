package sshkey_test

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aerospike-community/aerolab/pkg/backend/clouds/sshkey"
	"github.com/rglonek/logger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const testKeyName = "test-project"

func quietLogger() *logger.Logger {
	l := logger.NewLogger()
	l.SetLogLevel(logger.CRITICAL)
	return l
}

// publicKeyFor derives the public key from the stored private key, which is
// what an instance created with the returned public key has to authenticate
// against.
func publicKeyFor(t *testing.T, privatePath string) []byte {
	t.Helper()
	stored, err := os.ReadFile(privatePath)
	require.NoError(t, err)
	block, _ := pem.Decode(stored)
	require.NotNil(t, block, "private key is not valid PEM")
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)
	return bytes.Trim(ssh.MarshalAuthorizedKey(publicKey), "\n\r\t ")
}

// Concurrent instance creation used to leave every caller with a different key
// while only the last writer's private key survived on disk, so all but one of
// the created instances rejected the key they were built with.
func TestEnsureIsConsistentUnderConcurrency(t *testing.T) {
	dir := t.TempDir()
	log := quietLogger()

	const callers = 12
	keys := make([][]byte, callers)
	errs := make([]error, callers)
	wg := new(sync.WaitGroup)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys[i], errs[i] = sshkey.Ensure(dir, testKeyName, log)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "caller %d", i)
	}
	onDisk := publicKeyFor(t, filepath.Join(dir, testKeyName))
	for i, key := range keys {
		require.Equalf(t, string(onDisk), string(key), "caller %d was given a key that does not match the stored private key", i)
	}
}

func TestEnsureReusesStoredKey(t *testing.T) {
	dir := t.TempDir()
	log := quietLogger()

	first, err := sshkey.Ensure(dir, testKeyName, log)
	require.NoError(t, err)
	second, err := sshkey.Ensure(dir, testKeyName, log)
	require.NoError(t, err)

	require.Equal(t, string(first), string(second))
	require.Equal(t, string(publicKeyFor(t, filepath.Join(dir, testKeyName))), string(first))
}

func TestEnsureStoresTrimmedPublicKey(t *testing.T) {
	dir := t.TempDir()

	key, err := sshkey.Ensure(dir, testKeyName, quietLogger())
	require.NoError(t, err)
	require.NotEmpty(t, key)
	require.Equal(t, string(key), string(bytes.Trim(key, "\n\r\t ")))
	require.True(t, bytes.HasPrefix(key, []byte("ssh-rsa ")))
}

// A pair left half-written by a crash cannot be completed, since neither file
// can be derived from the other, so both are replaced.
// Two aerolab processes that share a root directory must not each generate a
// different key for the same project. This is the cross-process counterpart of
// TestEnsureIsConsistentUnderConcurrency.
func TestEnsureConsistentAcrossProcesses(t *testing.T) {
	if os.Getenv("SSHKEY_WORKER") == "1" {
		dir := os.Getenv("SSHKEY_DIR")
		outPath := os.Getenv("SSHKEY_OUT")
		require.NotEmpty(t, dir)
		require.NotEmpty(t, outPath)
		key, err := sshkey.Ensure(dir, testKeyName, quietLogger())
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(outPath, key, 0600))
		return
	}

	dir := t.TempDir()
	const workers = 4
	keys := make([][]byte, workers)
	wg := new(sync.WaitGroup)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outPath := filepath.Join(dir, fmt.Sprintf("worker-%d.key", i))
			cmd := exec.Command(os.Args[0], "-test.run=^TestEnsureConsistentAcrossProcesses$", "-test.count=1")
			cmd.Env = append(os.Environ(),
				"SSHKEY_WORKER=1",
				"SSHKEY_DIR="+dir,
				"SSHKEY_OUT="+outPath,
			)
			require.NoErrorf(t, cmd.Run(), "worker %d", i)
			key, err := os.ReadFile(outPath)
			require.NoErrorf(t, err, "worker %d", i)
			keys[i] = key
			require.NotEmptyf(t, keys[i], "worker %d returned no key", i)
		}(i)
	}
	wg.Wait()
	onDisk := publicKeyFor(t, filepath.Join(dir, testKeyName))
	for i, key := range keys {
		require.Equalf(t, string(onDisk), string(key), "worker %d was given a key that does not match the stored private key", i)
	}
}

func TestEnsureReplacesIncompletePair(t *testing.T) {
	dir := t.TempDir()
	log := quietLogger()
	privatePath := filepath.Join(dir, testKeyName)
	publicPath := privatePath + ".pub"

	first, err := sshkey.Ensure(dir, testKeyName, log)
	require.NoError(t, err)
	require.NoError(t, os.Remove(publicPath))

	second, err := sshkey.Ensure(dir, testKeyName, log)
	require.NoError(t, err)
	require.NotEqual(t, string(first), string(second))
	require.FileExists(t, publicPath)
	require.Equal(t, string(publicKeyFor(t, privatePath)), string(second))
}
