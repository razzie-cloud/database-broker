# Broker authentication and transport

Authentication defaults to `AUTH_MODE=token` (`--auth-mode`). Before initializing
any backend, startup requires `AUTH_POLICY_FILE`, `AUTH_DEPLOYMENT_ID` and
`AUTH_KEY_FILES=k1=/run/secrets/broker_master_k1` (matching flags use dashes).
Key files contain exactly 32 random raw bytes, never an encoded string. Comma
separated key_id=file pairs support at most four explicitly accepted keys.
`AUTH_MODE=none` explicitly preserves tokenless listing/provisioning, ignores
client/token query parameters and does not read policy/key files. It logs a warning.
Invalid/empty explicit modes and incomplete secure configuration fail closed.

Policy is one strict YAML document with version 1, deployment_id, key_id and
clients. Each client has a positive generation, exact allow adapter/instance
pairs and optional enabled (default true). Optional root accepted_key_ids and
per-client accepted_generations permit up to four unique values including the
active value; omission means only the active value. Clients use lowercase ASCII
letters, digits and underscores (first character a letter, at most 64 bytes).
Deployment/key IDs use ASCII letters, digits, underscore or hyphen (at most 64
bytes, first character alphanumeric). Instance grants are at most 128 bytes,
ASCII alphanumeric/underscore and case insensitive. Adapter names are case
insensitive; redis is the dragonfly alias. Duplicate canonical grants, unknown
fields, duplicate YAML keys and multiple documents are rejected.

Credential requests require exactly one `client` and `token` query parameter.
Both JSON and URI endpoints authenticate before backend lookup/provisioning;
listing is always forbidden to authenticated clients. Missing/invalid credentials
return generic 401; out-of-scope pairs return 403. Health endpoints are exempt.
Credential responses, including errors, have Cache-Control: no-store in both modes.

The token is `v1.<key_id>.<generation>.<MAC>`. MAC is full HMAC-SHA-256 encoded
as unpadded canonical base64url over compact JSON without trailing newline:
`["razcloud-broker",1,"deployment_id","key_id","client_id",generation]`.
Generation uses canonical decimal, from 1 through 9007199254740991. Comparisons
use crypto/hmac.Equal. Permissions are excluded from derivation and evaluated
against current policy on every request. Tokens have no automatic expiry.

The image retains /database-broker as entrypoint and includes /broker-token:

```
/broker-token validate --policy /policy.yml --deployment-id my-deployment
/broker-token generate --policy /policy.yml --deployment-id my-deployment --key-file /run/secrets/master --client files
/broker-token hold --max-lifetime 10m
```

Validate emits normalized public JSON and needs no key/database. Generate writes
exactly one token with no newline; use only a protected pipe to a secret store.
Diagnostics never echo supplied values. Hold handles SIGTERM/SIGINT and cannot
exceed 15 minutes. No command initializes provisioning backends or health monitors.

Set TLS_CERT_FILE / --tls-cert-file and TLS_KEY_FILE / --tls-key-file together
for verified HTTPS (minimum TLS 1.2). Consumers must trust the issuing CA and use
a hostname present in the certificate SANs. Issuance/renewal belongs to operator
PKI, independent of token keys. Never disable verification. HTTP remains useful
for isolated local development, but tokens do not protect against traffic capture.

To rotate a client, increment generation and temporarily include old/new accepted
values; distribute the new secret, converge consumers, then remove the old value
and converge every broker replica. Master rotation similarly uses a new key_id,
explicit old/new accepted_key_ids and both mounted key files, then removes old
acceptance after consumer convergence. Permission changes require only policy
rollout. Removal/revocation must converge all replicas. Do not roll back to a
revoked policy or delete resources still referenced by running services.

Back up Swarm state. Losing the master requires a new key version and token
redistribution; it does not require recreating application databases. Revoking a
broker token does not rotate database credentials already returned. Docker control
plane administrators can schedule tasks with access to secrets and remain trusted.
