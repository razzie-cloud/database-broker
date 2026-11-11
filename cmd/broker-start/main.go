// broker-start adapts legacy apps without allowing discovery failure to select memory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/razzie-cloud/database-broker/internal/discovery"
	"github.com/redis/go-redis/v9"
)

func main() {
	if run(os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "broker-start: credential discovery or backend validation failed")
		os.Exit(1)
	}
}
func run(args []string) error {
	bad := errors.New("invalid startup configuration")
	f := flag.NewFlagSet("broker-start", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	program := f.String("program", "", "absolute application path")
	target := f.String("url", "", "public broker discovery URL")
	token := f.String("token-file", "", "application token file")
	ca := f.String("ca-file", "", "broker CA file")
	if f.Parse(args) != nil || (*program != "/razchess" && *program != "/geoip-server") {
		return bad
	}
	for _, a := range f.Args() {
		if strings.HasPrefix(a, "-redis") || strings.HasPrefix(a, "--redis") {
			return bad
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	value, e := discovery.Fetch(ctx, *target, discovery.Options{TokenFile: *token, CAFile: *ca})
	if e != nil {
		return e
	}
	options, e := redis.ParseURL(value)
	if e != nil {
		return bad
	}
	options.DialTimeout = 3 * time.Second
	options.ReadTimeout = 3 * time.Second
	options.WriteTimeout = 3 * time.Second
	options.MaxRetries = 0
	r := redis.NewClient(options)
	e = r.Ping(ctx).Err()
	r.Close()
	if e != nil {
		return bad
	}
	// Discovery has already succeeded; bypass unsafe upstream HTTP code entirely.
	argv := append([]string{*program, "--redis", value}, f.Args()...)
	return syscall.Exec(*program, argv, os.Environ())
}
