export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public requestId: string | null,
    public fields: Record<string, string> = {},
  ) {
    super(message);
  }
}
export async function api<T = unknown>(
  path: string,
  body?: unknown,
  method?: string,
): Promise<T> {
  let r: Response;
  try {
    r = await fetch("/api/v1/" + path, {
      method: method ?? (body ? "POST" : "GET"),
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: body ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError(
      0,
      "NETWORK_ERROR",
      "Nu ne putem conecta la server. Verifică conexiunea și încearcă din nou.",
      null,
    );
  }
  const data = await r.json().catch(() => null);
  if (!r.ok || data === null) {
    if (r.status === 401 && path !== "auth/login")
      window.dispatchEvent(new Event("session-expired"));
    const e = data?.error;
    throw new ApiError(
      r.status,
      e?.code ?? "UNAVAILABLE",
      e?.message ?? "Serviciul nu este disponibil momentan. Încearcă din nou.",
      e?.requestId ?? r.headers.get("X-Request-ID"),
      e?.fields ?? {},
    );
  }
  return data as T;
}
