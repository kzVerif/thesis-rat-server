# REST origin behind HTTPS

Production uses a same-host Cloudflare tunnel or TLS reverse proxy. Go remains
HTTP on loopback; the Agent's public API endpoint must be HTTPS.

```dotenv
TRANSPORT_MODE=production
SERVER_HOST=127.0.0.1
SERVER_PORT=8080
FRONTEND_ORIGIN=https://lab.example.com
```

Set existing database/storage settings separately. This repository deliberately
reads .env rather than OS environment; do not assume exported variables override it.
The default mode remains development. Production fails before connecting to the DB
if the bind is not a literal loopback IP, or the frontend origin list is missing,
plaintext, wildcarded or malformed. IPv6 loopback ::1 is supported.

Cloudflare public api.example.com forwards to http://127.0.0.1:8080. If the proxy
crosses machines, use a TLS proxy on the origin host or another protected channel;
do not expose this plaintext listener. Require HTTPS at the public edge and bypass
caching for API/session/enrollment responses. Preserve request paths.

Dashboard browser /api/* stays routed to Next.js; its existing server-side proxy
can call this same-host loopback REST origin. Keep /ws/frontend on the Dashboard's
public hostname, routed to the WS service, to preserve the host-only session cookie.
__Host-session stays Secure, HttpOnly, SameSite=Lax, Path=/, without Domain.
No trusted proxy header is configured: client-IP auditing keeps the direct peer.
X-Forwarded-For/Proto/Host and CF-Connecting-IP must not grant authentication.

Enrollment tokens, quota transactions, public-key validation and /exists behavior
are unchanged. /exists only reports a record; it does not prove Agent ownership.
TLS validates the server; Agent Ed25519 proof remains future Phase 4.

Use the WS repository's deploy/cloudflared.example.yml as the shared topology
example. Test actual cookies/CORS/login/logout behind the real proxy before rollout.
