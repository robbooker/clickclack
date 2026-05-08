export class APIError extends Error {
  constructor(
    public status: number,
    body: string,
  ) {
    super(body);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body && !(init.body instanceof FormData))
    headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) {
    throw new APIError(response.status, await response.text());
  }
  return response.json() as Promise<T>;
}
