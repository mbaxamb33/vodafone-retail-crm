import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { api } from "../../api";
import { date, type Workspace } from "../../domain";
const schema = z.object({
  items: z.array(
    z.object({
      id: z.string(),
      customerId: z.string(),
      employeeId: z.string(),
      day: z.string(),
      status: z.enum(["open", "unreachable", "done"]),
    }),
  ),
});
export default function Experience({
  data,
  userId,
  onOpen,
}: {
  data: Workspace;
  userId: string;
  onOpen: (id: string) => void;
}) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["experience"],
    queryFn: async () => schema.parse(await api("experience")),
    refetchInterval: 60_000,
  });
  const save = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api("experience/" + encodeURIComponent(id), { status }, "PATCH"),
    onSuccess: () => qc.invalidateQueries(),
  });
  const items = q.data?.items ?? [];
  const remaining = items.filter((t) => t.status !== "done");
  const reminders = data.followUps
    .filter(
      (f) =>
        f.employeeId === userId && f.kind === "reminder" && f.status !== "done",
    )
    .sort((a, b) => a.due.localeCompare(b.due));
  return (
    <>
      <section className="card experience-card" id="experience">
        <div className="section-heading">
          <div>
            <span className="eyebrow">DUPĂ VIZITĂ · EXPERIENȚA CLIENTULUI</span>
            <h2>Cum a fost pentru ei?</h2>
            <p>Clienții de ieri și revenirile rămase de parcurs.</p>
          </div>
          <strong>{remaining.length} de contactat</strong>
        </div>
        {q.isPending && <p role="status">Se pregătește lista…</p>}
        {q.error && <p role="alert">{q.error.message}</p>}
        {save.error && (
          <p className="error" role="alert">
            {save.error.message}
          </p>
        )}
        {q.isSuccess && !remaining.length && (
          <p className="section-empty">
            Ești la zi. Ai parcurs toate revenirile de experiență.
          </p>
        )}
        {items.map((task) => {
          const c = data.customers.find((c) => c.id === task.customerId);
          return (
            <div
              key={task.id}
              className={
                "experience-row " + (task.status === "done" ? "completed" : "")
              }
            >
              <label className="check-label">
                <input
                  type="checkbox"
                  aria-label={`Experiență verificată pentru ${c?.phone || task.customerId}`}
                  checked={task.status === "done"}
                  disabled={save.isPending}
                  onChange={(e) =>
                    save.mutate({
                      id: task.id,
                      status: e.target.checked ? "done" : "open",
                    })
                  }
                />
                <span>
                  {task.status === "done" ? "Finalizat" : "De verificat"}
                </span>
              </label>
              <button
                className="task-main"
                onClick={() => onOpen(task.customerId)}
              >
                <strong>{c?.name || c?.phone || "Client"}</strong>
                <span>
                  Vizită {date(task.day)}
                  {task.status === "unreachable"
                    ? " · Nu a răspuns, reîncearcă"
                    : ""}
                </span>
              </button>
              {task.status !== "done" && (
                <div className="action-row">
                  <a className="button" href={"tel:" + c?.phone}>
                    Sună
                  </a>
                  <button
                    className="button subtle"
                    disabled={save.isPending}
                    onClick={() =>
                      save.mutate({ id: task.id, status: "unreachable" })
                    }
                  >
                    Nu a răspuns
                  </button>
                </div>
              )}
            </div>
          );
        })}
      </section>
      {reminders.length > 0 && (
        <section className="card experience-card">
          <span className="eyebrow">REMINDERE INTERNE</span>
          <h2>De verificat cu clientul</h2>
          {reminders.map((f) => {
            const c = data.customers.find((c) => c.id === f.customerId);
            return (
              <button
                className="experience-row reminder-link"
                key={f.id}
                onClick={() => onOpen(f.customerId)}
              >
                <strong>{date(f.due)}</strong>
                <span>
                  {c?.name || c?.phone}
                  <small className="reminder-context">{f.notes}</small>
                </span>
                <span>Deschide profilul →</span>
              </button>
            );
          })}
        </section>
      )}
    </>
  );
}
