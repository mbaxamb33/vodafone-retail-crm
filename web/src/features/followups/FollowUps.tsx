import { useState } from "react";
import { Check, CalendarClock } from "lucide-react";
import { Avatar, Empty } from "../../components/ui";
import {
  today,
  date,
  followUpStatuses,
  type Workspace,
  type User,
  type FollowUp,
} from "../../domain";
import FollowUpForm from "./FollowUpForm";
export default function FollowUps({
  data,
  user,
  onOpen,
  onComplete,
  pending,
}: {
  data: Workspace;
  user: User;
  onOpen: (id: string) => void;
  onComplete: (id: string) => void;
  pending: boolean;
}) {
  const [filter, setFilter] = useState("all");
  const [selected, setSelected] = useState<FollowUp | null>(null);
  const list = data.followUps
    .filter(
      (f) =>
        f.employeeId === user.id &&
        (filter === "done" ? f.status === "done" : f.status !== "done") &&
        (filter === "all" ||
          filter === "done" ||
          (filter === "late"
            ? f.due < today()
            : filter === "today"
              ? f.due === today()
              : filter === "upcoming"
                ? f.due > today()
                : f.status === filter)),
    )
    .sort((a, b) => a.due.localeCompare(b.due));
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">URMĂTORUL PAS CONTEAZĂ</span>
          <h1>Revino la momentul potrivit.</h1>
          <p>Promisiuni de păstrat și conversații de continuat.</p>
        </div>
      </div>
      <div className="tabs wrap-tabs">
        {[
          ["all", "Toate active"],
          ["late", "Restante"],
          ["today", "Astăzi"],
          ["upcoming", "Urmează"],
          ["waiting", "Așteptăm clientul"],
          ["unreachable", "Nu a răspuns"],
          ["done", "Finalizate"],
        ].map(([id, label]) => (
          <button
            className={filter === id ? "active" : ""}
            key={id}
            onClick={() => setFilter(id)}
          >
            {label}
          </button>
        ))}
      </div>
      <section className="card">
        {list.map((f) => {
          const c = data.customers.find((c) => c.id === f.customerId)!;
          return (
            <div className="task-row followup-row" key={f.id}>
              <Avatar name={c.name} />
              <button className="task-main" onClick={() => onOpen(c.id)}>
                <strong>{c.name}</strong>
                <span>{f.type}</span>
                <span className="followup-status">
                  {followUpStatuses[f.status]}
                  {f.opportunityId
                    ? " · " +
                      (data.opportunities.find((o) => o.id === f.opportunityId)
                        ?.product ?? "Oportunitate")
                    : ""}
                </span>
              </button>
              <span
                className={
                  "due-tag " +
                  (f.due < today() && f.status !== "done" ? "late" : "")
                }
              >
                {f.status === "done" && f.completedAt
                  ? "Finalizat " + date(f.completedAt)
                  : date(f.due)}
              </span>
              {f.status !== "done" && (
                <div className="action-row">
                  <button
                    className="button"
                    aria-label={"Actualizează follow-up pentru " + c.name}
                    onClick={() => setSelected(f)}
                  >
                    <CalendarClock size={16} />
                    Actualizează
                  </button>
                  <button
                    className="button subtle"
                    disabled={pending}
                    onClick={() => onComplete(f.id)}
                  >
                    <Check size={16} />
                    Finalizat
                  </button>
                </div>
              )}
            </div>
          );
        })}
        {!list.length && (
          <Empty>Nu ai follow-up-uri în această categorie.</Empty>
        )}
      </section>
      {selected && (
        <FollowUpForm
          customerId={selected.customerId}
          customerName={
            data.customers.find((c) => c.id === selected.customerId)?.name ?? ""
          }
          user={user}
          users={data.users}
          followUp={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </>
  );
}
