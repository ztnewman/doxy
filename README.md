# doxy

Reverse proxy that auto-routes `<container>.<domain>` to the running Docker container named `<container>`.

`hello.test.com` → the container named `test`.

## Usage

```sh
DOMAIN=test.com go run .
```

Or with Docker:

```sh
docker compose up -d --build
```

The proxy needs:
- Access to the Docker socket (`/var/run/docker.sock`)
- The same Docker network as the containers it proxies to (compose uses a network named `proxy` — attach your app containers to it)

## Port selection

For each container the target port is chosen as:

1. The `proxy.port=<port>` Docker label, if set.
2. Otherwise, the lowest exposed TCP port (`EXPOSE` in the image).

Example container:

```sh
docker run -d --name test --network proxy \
  --label proxy.port=8080 \
  myimage:latest
```

## DNS

Point a wildcard `*.test.com` A record at the proxy host, or add entries to `/etc/hosts` for local testing.

## Environment

| Var      | Default | Description                                        |
| -------- | ------- | -------------------------------------------------- |
| `DOMAIN` | —       | Required. Base domain. Routes match `*.<DOMAIN>`.  |
| `LISTEN` | `:80`   | HTTP listen address.                               |

Standard Docker client env vars (`DOCKER_HOST`, etc.) are honored.
