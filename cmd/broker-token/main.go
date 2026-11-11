// broker-token deliberately imports no backend or HTTP server packages.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/razzie-cloud/database-broker/internal/auth"
)

func main() {
	if run(os.Args[1:], os.Stdin, os.Stdout) != nil {
		fmt.Fprintln(os.Stderr, "broker-token: operation failed (check policy, client and mounted key)")
		os.Exit(1)
	}
}
func run(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 && args[0] == "probe" {
		return probe(args[1:])
	}
	bad := errors.New("invalid command")
	if len(args) == 0 {
		return bad
	}
	f := flag.NewFlagSet("broker-token", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	policy := f.String("policy", "", "policy file (- for validate stdin)")
	deployment := f.String("deployment-id", "", "expected deployment identity")
	key := f.String("key-file", "", "mounted 32-byte key file")
	client := f.String("client", "", "client identifier")
	lifetime := f.Duration("max-lifetime", 10*time.Minute, "hold lifetime (at most 15m)")
	if f.Parse(args[1:]) != nil || f.NArg() != 0 {
		return bad
	}
	if args[0] == "hold" {
		if *lifetime <= 0 || *lifetime > 15*time.Minute {
			return bad
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()
		timer := time.NewTimer(*lifetime)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		return nil
	}
	if args[0] != "validate" && args[0] != "generate" {
		return bad
	}
	var p *auth.Policy
	var err error
	if *policy == "-" && args[0] == "validate" {
		p, err = auth.Parse(stdin, *deployment)
	} else {
		p, err = auth.LoadPolicy(*policy, *deployment)
	}
	if err != nil {
		return err
	}
	if args[0] == "validate" {
		return json.NewEncoder(stdout).Encode(p)
	}
	k, err := auth.ReadKey(*key)
	if err != nil {
		return err
	}
	token, err := auth.Generate(p, k, *client)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, token)
	return err
}
