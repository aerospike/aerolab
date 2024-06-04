# Test Proxy Server

## Usage:
```
Usage:
  test-web-server [OPTIONS]

Application Options:
      --listen=           listen address; ignored if --tls is specified (listen in TLS is bound to 0.0.0.0:80+443) (default: 0.0.0.0:8080)
      --tls               enable TLS; this will ignore ListenAddr
      --tls-host=         autocert: specify domain to respond on; this parameter can be specified multiple times
      --tls-cache-dir=    autocert: directory to use for caching TLS certificates (default: tls-cache)
      --log-text-file=    path to a text file to log requests to (default: proxy.log)
      --log-json-file=    path to a json file to log requests to (default: proxy.json)
      --dest-url=         destination URL to send proxy requests to (default: http://127.0.0.1:3333/)
      --override-origin   if set, will override origin header to match --dest-url value
      --user-cookie-life= duration for which logged in user should remain logged in (default: 24h)
      --run-aerolab       if set, will also run aerolab webui; extra parameters can be specified as tail of the parameters (--x=y ... -- HERE)

Help Options:
  -h, --help              Show this help message
```

## WebServer

* will ask to enter a fake username; this username is passed as simulation to aerolab in `x-auth-aerolab-user` header; for tracking the proxy sets `proxy-fake-user` cookie
* handles all requests as a proxy, except for:
  * the `set fake username` page
  * the `/proxy-logout` endpoint, which logs the fake user out (reset tracking cookie)

## Example

```
./test-web-server.macos-amd64 --override-origin --run-aerolab -- --nobrowser --unique-firewalls --agi-strict-tls
```
