export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public requestId: string | null,
  ) {
    super(message);
  }
}
export async function api<T = unknown>(
  path: string,
  body?: unknown,
  method?: string,
): Promise<T> {
  const r = await fetch("/api/v1/" + path, {
    method: method ?? (body ? "POST" : "GET"),
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await r.json();
  if (!r.ok) {
    if (r.status === 401) window.dispatchEvent(new Event("session-expired"));
    throw new ApiError(
      r.status,
      data.error?.message ?? "A apărut o eroare. Încearcă din nou.",
      r.headers.get("X-Request-ID"),
    );
  }
  return data as T;
}
