# DNSPOD module for Caddy

This package contains a DNS provider module for [Caddy](https://github.com/caddyserver/caddy). It can be used to manage DNS records with DNSPOD accounts.

## Caddy module name

```
dns.providers.dnspod
```

## Config examples

To use this module for the ACME DNS challenge, [configure the ACME issuer in your Caddy JSON](https://caddyserver.com/docs/json/apps/tls/automation/policies/issuer/acme/) like so:

```json
{
  "module": "acme",
  "challenges": {
    "dns": {
      "provider": {
        "name": "dnspod",
        "auth_token": "{env.DNSPOD_TOKEN}"
      }
    }
  }
}
```

or with the Caddyfile:

```
tls {
  dns dnspod {env.DNSPOD_TOKEN}
}
```

You can replace `{env.DNSPOD_TOKEN}` with the actual auth token, which format is "APP_ID,APP_TOKEN", if you prefer to put it directly in your config instead of an environment variable.

## Authenticating

See [the associated README in the libdns package](https://github.com/libdns/dnspod) for important information about credentials.

**NOTE**: If migrating from Caddy v1, you will need to change from using a DNSPOD API Key to a scoped API Token. Please see link above for more information.
