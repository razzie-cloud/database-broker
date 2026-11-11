#!/usr/bin/env python3
"""Generate independent ignored development credentials using the shared CLI."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import sys


def main():
    root = Path(__file__).resolve().parents[1]
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--policy', type=Path, default=root / 'dev/broker-clients.yml')
    p.add_argument('--directory', type=Path, default=root / '.local/broker-auth')
    p.add_argument('--deployment-id', default='broker-development')
    args = p.parse_args()
    if not args.deployment_id.endswith('-development'):
        raise ValueError('Only development deployments are permitted')
    directory = args.directory.resolve()
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    key = directory / 'master'
    with tempfile.TemporaryDirectory(prefix='broker-dev-cli-') as temp:
        binary = str(Path(temp) / 'broker-token')
        subprocess.run(['go', 'build', '-o', binary, './cmd/broker-token'], cwd=root, check=True)
        def cli(*values):
            return subprocess.run([binary, *values, '--policy', str(args.policy.resolve()),
                                   '--deployment-id', args.deployment_id], check=True, capture_output=True).stdout
        policy = json.loads(cli('validate'))
        if not key.exists():
            if any(directory.iterdir()):
                raise ValueError('Development master is missing; choose a fresh isolated directory')
            fd = os.open(key, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'wb') as f:
                f.write(os.urandom(32))
        for client, config in sorted(policy['clients'].items()):
            if config.get('enabled') is False:
                continue
            token = cli('generate', '--key-file', str(key), '--client', client)
            # Read-only app mounts must be readable by UID 10001. The containing
            # host directory remains private; these are disposable local credentials.
            with tempfile.NamedTemporaryFile(dir=directory, delete=False) as f:
                name = f.name
                f.write(token)
                os.fchmod(f.fileno(), 0o444)
            os.replace(name, directory / (client + '.token'))
    print('Development broker credentials prepared (never use them in production).')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.CalledProcessError):
        print('Development credential preparation failed; check local policy, Go and ignored directory.', file=sys.stderr)
        sys.exit(1)
