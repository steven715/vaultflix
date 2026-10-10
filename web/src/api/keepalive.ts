// postKeepalive sends a JSON POST that survives page teardown (fetch keepalive),
// for the final reports sent while a page is unmounting or closing. The axios
// client can't do this, so auth is attached here the same way it is there.
export function postKeepalive(path: string, body: unknown): void {
  const token = localStorage.getItem('token')
  fetch(`/api${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    keepalive: true,
  }).catch((err) => console.warn(`keepalive POST ${path} failed`, err))
}
