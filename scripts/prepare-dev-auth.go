//go:build ignore

// Run from database-broker with go run scripts/prepare-dev-auth.go.
// Credentials are disposable, independent of production, and kept in .local.
package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/razzie-cloud/database-broker/internal/auth"
)

func prepare() error {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(source))
	policyPath := flag.String("policy", filepath.Join(root, "dev/broker-clients.yml"), "development policy")
	directory := flag.String("directory", filepath.Join(root, ".local/broker-auth"), "ignored credentials directory")
	deployment := flag.String("deployment-id", "broker-development", "development deployment identity")
	flag.Parse()
	if flag.NArg() != 0 || !strings.HasSuffix(*deployment, "-development") {
		return errors.New("only development deployments are permitted")
	}
	policy, err := auth.LoadPolicy(*policyPath, *deployment)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(*directory, 0700); err != nil {
		return err
	}
	keyPath := filepath.Join(*directory, "master")
	if _, err = os.Stat(keyPath); os.IsNotExist(err) {
		entries, err := os.ReadDir(*directory)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return errors.New("master missing from nonempty directory")
		}
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		f, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	} else if err != nil {
		return err
	}
	key, err := auth.ReadKey(keyPath)
	if err != nil {
		return err
	}
	clients := make([]string, 0, len(policy.Clients))
	for c := range policy.Clients {
		clients = append(clients, c)
	}
	sort.Strings(clients)
	for _, client := range clients {
		if !policy.Clients[client].IsEnabled() {
			continue
		}
		token, err := auth.Generate(policy, key, client)
		if err != nil {
			return err
		}
		if err = writeToken(*directory, client, token); err != nil {
			return err
		}
	}
	fmt.Println("Development broker credentials prepared (never use them in production).")
	return nil
}
func writeToken(directory, client, token string) error {
	f, err := os.CreateTemp(directory, ".token-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.WriteString(token); err != nil {
		return err
	}
	// The host directory is private; app mounts must be readable by UID 10001.
	if err = f.Chmod(0444); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(directory, client+".token"))
}
func main() {
	if prepare() != nil {
		fmt.Fprintln(os.Stderr, "Development credential preparation failed; check local policy, Go and ignored directory.")
		os.Exit(1)
	}
}
