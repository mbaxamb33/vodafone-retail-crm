import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react";
import { api } from "../../api";
import { date, notificationsSchema } from "../../domain";

export default function Notifications({
  onOpen,
}: {
  onOpen: (customerId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["notifications"],
    queryFn: async () => notificationsSchema.parse(await api("notifications")),
    refetchInterval: 60_000,
  });
  const read = useMutation({
    mutationFn: (ids: string[]) => api("notifications/read", { ids }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    return () => document.removeEventListener("mousedown", close);
  }, [open]);
  const unread = q.data?.unread ?? 0;
  return (
    <div className="notifications" ref={ref}>
      <button
        className="icon-button"
        aria-label={
          unread ? `Notificări, ${unread} necitite` : "Notificări, niciuna nouă"
        }
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <Bell size={19} />
        {unread > 0 && <span className="nav-count">{unread}</span>}
      </button>
      {open && (
        <div
          className="notifications-panel"
          role="dialog"
          aria-label="Notificări"
        >
          <div className="section-heading">
            <strong>Noutăți pentru tine</strong>
            {unread > 0 && (
              <button
                className="text-link"
                disabled={read.isPending}
                onClick={() => read.mutate([])}
              >
                <CheckCheck size={15} />
                Marchează tot ca citit
              </button>
            )}
          </div>
          {q.isError && <p className="error">{q.error.message}</p>}
          {q.data?.items.map((n) => (
            <button
              key={n.id}
              className={"notification-row " + (n.readAt ? "" : "unread")}
              onClick={() => {
                if (!n.readAt) read.mutate([n.id]);
                if (n.customerId) onOpen(n.customerId);
                setOpen(false);
              }}
            >
              {!n.readAt && <span className="red-dot" aria-label="Necitită" />}
              <span>
                <strong>{n.customerName || "Client"}</strong>
                <small>{n.message}</small>
              </span>
              <time>{date(n.createdAt)}</time>
            </button>
          ))}
          {q.data && !q.data.items.length && (
            <p className="section-empty">Nicio notificare deocamdată.</p>
          )}
        </div>
      )}
    </div>
  );
}
