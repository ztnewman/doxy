<p align="center">
  <img src="doxy.png" alt="doxy" width="220">
</p>

<h1 align="center">doxy</h1>

<p align="center">
  A tiny reverse proxy that auto-routes <code>&lt;container&gt;.&lt;domain&gt;</code> to running Docker containers — zero config per container.
</p>

<p align="center">
  <a href="https://github.com/ztnewman/doxy/blob/main/go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/ztnewman/doxy?style=flat-square&logo=go&logoColor=white&color=00ADD8"></a>
  <a href="https://github.com/ztnewman/doxy/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/github/license/ztnewman/doxy?style=flat-square&color=14b8a6"></a>
  <a href="https://www.docker.com/"><img alt="Docker" src="https://img.shields.io/badge/docker-ready-2496ed?style=flat-square&logo=docker&logoColor=white"></a>
  <a href="https://github.com/ztnewman/doxy/commits/main"><img alt="Last commit" src="https://img.shields.io/github/last-commit/ztnewman/doxy?style=flat-square&color=ea7c1f"></a>
  <a href="https://github.com/ztnewman/doxy/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/ztnewman/doxy?style=flat-square&color=64748b"></a>
</p>

---

`hello.test.com` → the container named `hello`. No labels, no config files, no restarts. doxy watches the Docker socket and updates its routing table live as containers start and stop.

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
docker run -d --name hello --network proxy \
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

## License

[MIT](LICENSE)
