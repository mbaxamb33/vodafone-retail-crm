import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  Phone,
  Plus,
  CalendarDays,
  MessageSquare,
  ShieldCheck,
  ArrowUpRight,
} from "lucide-react";
import { Avatar, Empty } from "../../components/ui";
import { api } from "../../api";
import {
  profileSchema,
  steps,
  stages,
  date,
  activeOpportunity,
  nextOpportunityAction,
  followUpStatuses,
  type Workspace,
  type User,
  type Customer,
  type Opportunity,
  type FollowUp,
} from "../../domain";
import OwnershipEditor from "./OwnershipEditor";
import { useNextStepLabel } from "../../hooks";
import FollowUpForm from "../followups/FollowUpForm";
export default function Profile({
  id,
  data,
  user,
  onClose,
  onVisit,
}: {
  id: string;
  data: Workspace;
  user: User;
  onClose: () => void;
  onVisit: (c: Customer) => void;
}) {
  const [offset, setOffset] = useState(0);
  const stepLabel = useNextStepLabel();
  const [tab, setTab] = useState("history");
  const [action, setAction] = useState<{
    opportunity?: Opportunity;
    followUp?: FollowUp;
  } | null>(null);
  const q = useQuery({
    queryKey: ["profile", id, offset],
    queryFn: async () =>
      profileSchema.parse(await api("customers/" + id + "?offset=" + offset)),
  });
  if (q.isPending)
    return (
      <div className="skeleton">
        <div />
        <div />
      </div>
    );
  if (q.error)
    return (
      <section className="card error-panel">
        <h2>Profilul nu a putut fi încărcat.</h2>
        <p className="error">{q.error.message}</p>
        <button className="button" onClick={() => q.refetch()}>
          Încearcă din nou
        </button>
        <button className="button" onClick={onClose}>
          Înapoi la clienți
        </button>
      </section>
    );
  const { customer, opportunities, followUps, lastVisit } = q.data;
  const active = opportunities.filter(activeOpportunity);
  const upcoming = followUps
    .filter((f) => f.status !== "done")
    .sort((a, b) => a.due.localeCompare(b.due));
  const ownerName = (id: string) =>
    data.users.find((u) => u.id === id)?.name ?? "Colegul responsabil";
  return (
    <>
      <button className="back-link" onClick={onClose}>
        <ArrowLeft size={17} />
        Înapoi la clienți
      </button>
      <div className="page-heading profile-page-heading">
        <div className="profile-hero">
          <Avatar name={customer.name || customer.phone} />
          <div>
            <span className="eyebrow">RELAȚIA CU CLIENTUL</span>
            <h1>{customer.name || customer.phone}</h1>
            {customer.status !== "active" && (
              <span className="tag">
                {customer.status === "archived" ? "Arhivat" : "Anonimizat"}
              </span>
            )}
            <a href={"tel:" + customer.phone}>
              <Phone size={16} />
              {customer.phone}
            </a>
          </div>
        </div>
        <button className="button primary" onClick={() => onVisit(customer)}>
          <Plus size={18} />
          Înregistrează vizita
        </button>
      </div>
      <div className="profile-layout">
        <div className="profile-primary">
          <section className="card context-card">
            <span className="eyebrow">UNDE AM RĂMAS</span>
            {lastVisit ? (
              <>
                <div className="section-heading">
                  <h2>{lastVisit.reason || "Ultima conversație"}</h2>
                  <span className="tag">{date(lastVisit.at)}</span>
                </div>
                <p className="context-author">
                  {ownerName(lastVisit.employeeId)} · Ultima vizită
                </p>
                <p className="context-note">
                  {lastVisit.notes ||
                    "Nu au fost adăugate note la ultima vizită."}
                </p>
                <div className="tags">
                  {lastVisit.steps.map((s) => (
                    <span className="tag" key={s}>
                      {steps[s]}
                    </span>
                  ))}
                </div>
              </>
            ) : (
              <>
                <h2>Prima conversație începe aici.</h2>
                <p>
                  Înregistrează o vizită pentru a păstra contextul relației.
                </p>
              </>
            )}
          </section>
          <section className="card profile-opportunities">
            <div className="section-heading">
              <div>
                <h2>Conversații comerciale</h2>
                <p>{active.length} oportunități active</p>
              </div>
            </div>
            {opportunities.map((o) => {
              const next = nextOpportunityAction(o, followUps);
              return (
                <article key={o.id} className="profile-opportunity">
                  <div className="section-heading">
                    <h3>{o.product}</h3>
                    <span
                      className={"tag " + (o.stage === "won" ? "positive" : "")}
                    >
                      {stages[o.stage]}
                    </span>
                  </div>
                  <p className="context-author">
                    Responsabil: {ownerName(o.employeeId)}
                  </p>
                  {activeOpportunity(o) &&
                    (next ? (
                      <div className="next-action-detail">
                        <CalendarDays size={16} />
                        <span>
                          <strong>{next.type}</strong>
                          <small>
                            {date(next.due)} · {ownerName(next.employeeId)} ·{" "}
                            {followUpStatuses[next.status]}
                          </small>
                        </span>
                        {(user.role === "manager" ||
                          next.employeeId === user.id) && (
                          <button
                            className="text-link"
                            onClick={() => setAction({ followUp: next })}
                          >
                            Actualizează
                            <ArrowUpRight size={15} />
                          </button>
                        )}
                      </div>
                    ) : (
                      <div className="missing-action">
                        <span>
                          {o.nextStep
                            ? `Stabilit cu clientul: ${stepLabel(o.nextStep)} · fără dată`
                            : "Fără pas următor"}
                        </span>
                        {(user.role === "manager" ||
                          o.employeeId === user.id) && (
                          <button
                            className="text-link"
                            onClick={() => setAction({ opportunity: o })}
                          >
                            Programează
                            <Plus size={15} />
                          </button>
                        )}
                      </div>
                    ))}
                </article>
              );
            })}
            {!opportunities.length && (
              <p className="section-empty">
                Nicio oportunitate înregistrată. Poți adăuga una în timpul
                vizitei.
              </p>
            )}
          </section>
          <section className="card history-card">
            <div className="tabs">
              <button
                className={tab === "history" ? "active" : ""}
                onClick={() => setTab("history")}
              >
                Istoric vizite ({q.data.total})
              </button>
              {user.role === "manager" && (
                <button
                  className={tab === "audit" ? "active" : ""}
                  onClick={() => setTab("audit")}
                >
                  Audit
                </button>
              )}
            </div>
            <div className="timeline">
              {tab === "history"
                ? q.data.visits.map((v) => (
                    <article key={v.id}>
                      <span className="timeline-dot">
                        <MessageSquare size={15} />
                      </span>
                      <div className="timeline-meta">
                        <strong>{ownerName(v.employeeId)}</strong>
                        <time>{date(v.at)}</time>
                      </div>
                      <h3>{v.reason || "Vizită în magazin"}</h3>
                      <div className="tags">
                        {v.steps.map((s) => (
                          <span key={s} className="tag">
                            {steps[s]}
                          </span>
                        ))}
                      </div>
                      {v.notes && <p className="visit-note">{v.notes}</p>}
                      {v.details?.nextAction && (
                        <div className="visit-detail-panel">
                          {v.details.resolution && (
                            <p>
                              <strong>Solicitare:</strong>{" "}
                              {v.details.resolution.type === "invoice"
                                ? `Încasare factură · ${v.details.resolution.holder === "holder" ? "Titular" : "Netitular"}`
                                : `Alte solicitări · ${v.details.resolution.status === "resolved" ? "Rezolvat" : v.details.resolution.status === "pending" ? "În așteptare · are caz deschis" : "Nerezolvat"}`}
                            </p>
                          )}
                          <p>
                            <strong>Stabilit cu clientul:</strong>{" "}
                            {v.details.nextActionLabel}
                            {v.details.actionDetails &&
                              ` · ${v.details.actionDetails}`}
                          </p>
                          {v.details.contactConsent && (
                            <p>Acord de contact exprimat în această vizită.</p>
                          )}
                          <p>
                            {v.details.agreedDate
                              ? `Dată stabilită cu clientul: ${date(v.details.due)}`
                              : "Fără dată stabilită cu clientul."}
                          </p>
                          {q.data.followUps
                            .filter(
                              (f) =>
                                f.kind === "reminder" &&
                                f.sourceVisitId === v.id,
                            )
                            .map((f) => (
                              <p key={f.id}>
                                <strong>
                                  Reminder intern · {date(f.due)}:
                                </strong>{" "}
                                {f.notes}
                              </p>
                            ))}
                        </div>
                      )}
                    </article>
                  ))
                : q.data.audit.map((a) => (
                    <article key={a.id}>
                      <span className="timeline-dot">
                        <ShieldCheck size={15} />
                      </span>
                      <div className="timeline-meta">
                        <strong>{ownerName(a.actorId)}</strong>
                        <time>{date(a.at)}</time>
                      </div>
                      <p>{a.detail}</p>
                      <small>{a.action}</small>
                    </article>
                  ))}
              {tab === "history" && !q.data.visits.length && (
                <Empty>Istoricul va apărea după prima vizită.</Empty>
              )}
              {tab === "audit" && !q.data.audit.length && (
                <p>Nicio schimbare înregistrată.</p>
              )}
            </div>
            {tab === "history" && q.data.total > 20 && (
              <div className="pagination">
                <button
                  className="button"
                  disabled={offset === 0}
                  onClick={() => setOffset(offset - 20)}
                >
                  Mai recente
                </button>
                <span>
                  {offset + 1}–{Math.min(offset + 20, q.data.total)} din{" "}
                  {q.data.total}
                </span>
                <button
                  className="button"
                  disabled={offset + 20 >= q.data.total}
                  onClick={() => setOffset(offset + 20)}
                >
                  Mai vechi
                </button>
              </div>
            )}
          </section>
        </div>
        <aside className="profile-side">
          <section className="card">
            <OwnershipEditor
              key={customer.id}
              customer={customer}
              user={user}
              users={data.users}
            />
            {q.data.ownershipHistory.length > 0 && (
              <ol
                className="ownership-history"
                aria-label="Istoric responsabil"
              >
                {q.data.ownershipHistory.map((h) => (
                  <li key={h.id}>
                    <time>{date(h.at)}</time>
                    <span>
                      {h.data?.toOwnerId
                        ? "Alocat lui " + ownerName(String(h.data.toOwnerId))
                        : h.detail}
                    </span>
                  </li>
                ))}
              </ol>
            )}
          </section>
          <section className="card next-steps-card">
            <div className="section-heading">
              <h2>Următorii pași</h2>
              <CalendarDays size={19} />
            </div>
            {upcoming.map((f) => (
              <article className="customer-next-step" key={f.id}>
                <span className="tag">{followUpStatuses[f.status]}</span>
                <h3>{f.type}</h3>
                <p>
                  {date(f.due)} · {ownerName(f.employeeId)}
                </p>
                {(user.role === "manager" || f.employeeId === user.id) && (
                  <button
                    className="text-link"
                    onClick={() => setAction({ followUp: f })}
                  >
                    Actualizează
                  </button>
                )}
              </article>
            ))}
            {!upcoming.length && (
              <p className="section-empty">
                Niciun pas programat pentru acest client.
              </p>
            )}
            <button className="button full" onClick={() => setAction({})}>
              <Plus size={16} />
              Programează un follow-up
            </button>
          </section>
        </aside>
      </div>
      {action && (
        <FollowUpForm
          customerId={id}
          customerName={customer.name || customer.phone}
          opportunityId={action.opportunity?.id}
          followUp={action.followUp}
          user={user}
          users={data.users}
          onClose={() => setAction(null)}
        />
      )}
    </>
  );
}
