// Shared helpers for the Nebius Rancher UI extension.
//
// All cloud calls go through the Rancher proxy (/meta/proxy/<host>/...), so the
// hosts must be present in the NodeDriver whitelistDomains.
//
// ponytail: the IAM token is obtained via the OAuth2 token-exchange endpoint
// (the same grant the Go SDK sends over gRPC); a token is kept in memory only.
// If the endpoint rejects the exchange the forms fall back to free-text input.

export const DEFAULT_API_ENDPOINT = 'api.nebius.cloud:443';
// Fixed IAM auth host used by github.com/nebius/gosdk; there is no driver
// flag for it, so token exchange always targets this host.
export const AUTH_ENDPOINT = 'auth.eu.nebius.com:443';

const TOKEN_PATH = '/oauth2/token';
// Audience sent by github.com/nebius/gosdk auth.ServiceAccountExchangeTokenRequester.
const JWT_AUDIENCE = 'token-service.iam.new.nebiuscloud.net';

export function splitEndpoint(endpoint) {
  const value = (endpoint || '').trim().replace(/^https?:\/\//, '').replace(/\/.*$/, '');

  if (!value) {
    return { host: '', port: '' };
  }

  const idx = value.lastIndexOf(':');

  if (idx === -1) {
    return { host: value, port: '' };
  }

  return { host: value.slice(0, idx), port: value.slice(idx + 1) };
}

export function endpointHost(endpoint) {
  return splitEndpoint(endpoint).host;
}

// The Rancher proxy takes a bare hostname; default HTTPS ports are omitted.
export function proxyHost(endpoint) {
  const { host, port } = splitEndpoint(endpoint);

  if (!port || port === '443') {
    return host;
  }

  return `${ host }:${ port }`;
}

export function isHostAllowed(driver, endpoint) {
  const domains = driver?.whitelistDomains || driver?.spec?.whitelistDomains || [];

  if (!domains.length) {
    return true;
  }

  const host = endpointHost(endpoint);

  return !!host && domains.includes(host);
}

function statusOf(e) {
  return e?._status ?? e?.status ?? e?.response?.status ?? 0;
}

export function errorText(t, e, fallbackKey) {
  const status = statusOf(e);
  const key =
    status === 401 ? 'cluster.nebius.errors.unauthorized'
      : status === 403 ? 'cluster.nebius.errors.forbidden'
        : status === 404 ? 'cluster.nebius.errors.notFound'
          : status === 429 ? 'cluster.nebius.errors.rateLimited'
            : status >= 500 ? 'cluster.nebius.errors.server'
              : null;

  if (key) {
    return t(key);
  }

  return e?.message || t(fallbackKey);
}

function pemToArrayBuffer(pem) {
  const body = (pem || '')
    .replace(/-----BEGIN [^-]+-----/g, '')
    .replace(/-----END [^-]+-----/g, '')
    .replace(/\s+/g, '');

  if (!body) {
    throw new Error('invalid-key');
  }

  const binary = atob(body);
  const bytes = new Uint8Array(binary.length);

  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }

  return bytes.buffer;
}

function base64url(input) {
  const bytes = new Uint8Array(input);
  let binary = '';

  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }

  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export async function signServiceAccountJWT({ privateKey, saId, publicKeyId, ttl = 60 }) {
  if (!privateKey || !/-----BEGIN (RSA )?PRIVATE KEY-----/.test(privateKey)) {
    throw new Error('invalid-key');
  }
  if (/-----BEGIN RSA PRIVATE KEY-----/.test(privateKey)) {
    throw new Error('pkcs1-key');
  }

  const key = await globalThis.crypto.subtle.importKey(
    'pkcs8',
    pemToArrayBuffer(privateKey),
    { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' },
    false,
    ['sign']
  );

  const now = Math.floor(Date.now() / 1000);
  const header = { alg: 'RS256', typ: 'JWT', kid: publicKeyId };
  const claims = {
    iss: saId, sub: saId, aud: JWT_AUDIENCE, iat: now, exp: now + ttl
  };
  const encoder = new TextEncoder();
  const input = `${ base64url(encoder.encode(JSON.stringify(header))) }.${ base64url(encoder.encode(JSON.stringify(claims))) }`;
  const signature = await globalThis.crypto.subtle.sign('RSASSA-PKCS1-v1_5', key, encoder.encode(input));

  return `${ input }.${ base64url(signature) }`;
}

export async function exchangeToken(store, { assertion }) {
  const host = proxyHost(AUTH_ENDPOINT);
  const body = new URLSearchParams({
    grant_type:          'urn:ietf:params:oauth:grant-type:token-exchange',
    requested_token_type:'urn:ietf:params:oauth:token-type:access_token',
    subject_token:       assertion,
    subject_token_type:  'urn:ietf:params:oauth:token-type:jwt',
  }).toString();

  const res = await store.dispatch('management/request', {
    method:              'POST',
    url:                 `/meta/proxy/${ host }${ TOKEN_PATH }`,
    headers:             {
      Accept:         'application/json',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    data:                body,
    redirectUnauthorized:false,
  }, { root: true });

  if (!res?.access_token) {
    throw new Error('no-token');
  }

  return res.access_token;
}

export async function proxyGet(store, { apiEndpoint, token, path, query = {} }) {
  const params = new URLSearchParams(query);

  Object.keys(query).forEach((k) => {
    if (query[k] === undefined || query[k] === null || query[k] === '') {
      params.delete(k);
    }
  });

  const host = proxyHost(apiEndpoint);
  const base = `/meta/proxy/${ host }${ path }`;
  const url = params.toString() ? `${ base }?${ params.toString() }` : base;
  const headers = { Accept: 'application/json' };

  if (token) {
    headers.Authorization = `Bearer ${ token }`;
  }

  return store.dispatch('management/request', {
    method: 'GET', url, headers, redirectUnauthorized: false,
  }, { root: true });
}

export function itemsOf(res) {
  if (Array.isArray(res)) {
    return res;
  }

  return Array.isArray(res?.items) ? res.items : [];
}

export function idOf(x) {
  return x?.id || x?.metadata?.id || x?.name || x?.metadata?.name || '';
}

export function nameOf(x) {
  return x?.name || x?.metadata?.name || x?.id || x?.metadata?.id || '';
}

export function resourceOption(x) {
  const name = nameOf(x);
  const id = idOf(x);

  return { label: name && id && name !== id ? `${ name } (${ id })` : (id || name), value: id || name };
}

// Cloud credential secrets use the <driver>credentialConfig-<field> key shape.
export function decodeCredential(secret) {
  const decode = (field) => {
    try {
      return atob(secret?.data?.[`nebiuscredentialConfig-${ field }`] || '');
    } catch (e) {
      return '';
    }
  };

  return {
    saId:              decode('saId'),
    publicKeyId:       decode('publicKeyId'),
    privateKey:        decode('privateKey'),
    projectId:         decode('projectId'),
    apiEndpoint:       decode('apiEndpoint') || DEFAULT_API_ENDPOINT,
  };
}
