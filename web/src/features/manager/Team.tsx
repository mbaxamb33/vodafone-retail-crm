import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { ArrowRight, BriefcaseBusiness, CalendarDays } from "lucide-react";
import { Avatar, Empty } from "../../components/ui";
import {
  date,
  today,
  stages,
  activeOpportunity,
  nextOpportunityAction,
  type Workspace,
  type User,
  type Customer,
  type FollowUp,
} from "../../domain";
import OwnershipEditor from "../customers/OwnershipEditor";
import FollowUpForm from "../followups/FollowUpForm";
import { useStoreReport } from "./reporting";
export default function Team({
  data,
  user,
  onOpen,
}: {
  data: Workspace;
  user: User;
  onOpen: (id: string) => void;
}) {
  const [params, setParams] = useSearchParams();
  const [selectedCustomer, setSelectedCustomer] = useState<Customer | null>(
    null,
  );
  const [task, setTask] = useState<FollowUp | null>(null);
  const [tab, setTab] = useState("portfolio");
  const { query: q, controls, valid } = useStoreReport();
  const members = data.users;
  const id = params.get("employee") ?? members[0]?.id;
  const employee = members.find((u) => u.id === id);
  const customers = data.customers.filter((c) => c.ownerId === id);
  const opportunities = data.opportunities.filter(
    (o) => o.employeeId === id && activeOpportunity(o),
  );
  const tasks = data.followUps
    .filter((f) => f.employeeId === id && f.status !== "done")
    .sort((a, b) => a.due.localeCompare(b.due));
  const visits = q.data?.visits.filter((v) => v.employeeId === id) ?? [];
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">MAGAZIN · ECHIPA MEA</span>
          <h1>Oamenii din spatele relațiilor.</h1>
          <p>
            Înțelege volumul de lucru și ajută fiecare coleg să facă următorul
            pas.
          </p>
        </div>
      </div>
      <div className="team-layout">
        <section className="team-roster" aria-label="Selectează un coleg">
          {members.map((member) => {
            const late = data.followUps.filter(
              (f) =>
                f.employeeId === member.id &&
                f.status !== "done" &&
                f.due < today(),
            ).length;
            return (
              <button
                className={
                  "team-member " + (id === member.id ? "selected" : "")
                }
                key={member.id}
                onClick={() => {
                  setParams({ employee: member.id });
                  setSelectedCustomer(null);
                }}
              >
                <Avatar name={member.name} />
                <span>
                  <strong>{member.name}</strong>
                  <small>
                    {member.role === "manager" ? "Manager" : "Consultant"} ·{" "}
                    {
                      data.customers.filter((c) => c.ownerId === member.id)
                        .length
                    }{" "}
                    clienți
                  </small>
                  <small className={late ? "needs-attention" : ""}>
                    {late
                      ? late + " reveniri restante"
                      : "Fără reveniri restante"}
                  </small>
                </span>
                <ArrowRight size={16} />
              </button>
            );
          })}
        </section>
        <div className="team-detail">
          {!employee ? (
            <Empty>Alege un coleg din listă.</Empty>
          ) : (
            <>
              <section className="card employee-summary">
                <div className="section-heading">
                  <div className="employee-identity">
                    <Avatar name={employee.name} />
                    <div>
                      <span className="eyebrow">
                        PORTOFOLIU ȘI VOLUM DE LUCRU
                      </span>
                      <h2>{employee.name}</h2>
                    </div>
                  </div>
                </div>
                <div className="employee-metrics">
                  <div>
                    <strong>{customers.length}</strong>
                    <span>Clienți alocați</span>
                  </div>
                  <div>
                    <strong>{opportunities.length}</strong>
                    <span>Oportunități active</span>
                  </div>
                  <div>
                    <strong>{tasks.length}</strong>
                    <span>Reveniri de făcut</span>
                  </div>
                  <div>
                    <strong className="needs-attention">
                      {tasks.filter((f) => f.due < today()).length}
                    </strong>
                    <span>Restante</span>
                  </div>
                </div>
              </section>
              <div className="tabs wrap-tabs">
                {[
                  ["portfolio", "Portofoliu"],
                  ["pipeline", "Oportunități"],
                  ["tasks", "Follow-up-uri"],
                  ["activity", "Activitate"],
                ].map(([value, label]) => (
                  <button
                    key={value}
                    className={tab === value ? "active" : ""}
                    onClick={() => setTab(value)}
                  >
                    {label}
                  </button>
                ))}
              </div>
              {tab === "portfolio" && (
                <section className="card">
                  {customers.map((c) => (
                    <div key={c.id} className="team-customer-row">
                      <Avatar name={c.name} small />
                      <button
                        className="task-main"
                        onClick={() => onOpen(c.id)}
                      >
                        <strong>{c.name}</strong>
                        <span>{c.phone}</span>
                      </button>
                      <button
                        className="button"
                        onClick={() =>
                          setSelectedCustomer(
                            selectedCustomer?.id === c.id ? null : c,
                          )
                        }
                      >
                        Reasignează
                      </button>
                    </div>
                  ))}
                  {!customers.length && (
                    <Empty>Acest coleg nu are clienți alocați.</Empty>
                  )}
                  {selectedCustomer && (
                    <OwnershipEditor
                      key={selectedCustomer.id}
                      customer={
                        data.customers.find(
                          (c) => c.id === selectedCustomer.id,
                        ) ?? selectedCustomer
                      }
                      user={user}
                      users={members}
                      onSaved={() => setSelectedCustomer(null)}
                    />
                  )}
                </section>
              )}
              {tab === "pipeline" && (
                <section className="card">
                  {opportunities.map((o) => {
                    const c = data.customers.find(
                      (c) => c.id === o.customerId,
                    )!;
                    const next = nextOpportunityAction(o, data.followUps);
                    return (
                      <button
                        className="employee-opportunity"
                        key={o.id}
                        onClick={() => onOpen(c.id)}
                      >
                        <BriefcaseBusiness size={20} />
                        <span>
                          <strong>{o.product}</strong>
                          <small>{c.name}</small>
                          <small>
                            {next
                              ? next.type + " · " + date(next.due)
                              : "Fără pas următor"}
                          </small>
                        </span>
                        <span className="tag">{stages[o.stage]}</span>
                        <ArrowRight size={17} />
                      </button>
                    );
                  })}
                  {!opportunities.length && (
                    <Empty>Nicio oportunitate activă.</Empty>
                  )}
                </section>
              )}
              {tab === "tasks" && (
                <section className="card">
                  {tasks.map((f) => (
                    <div className="team-customer-row" key={f.id}>
                      <CalendarDays size={20} />
                      <button
                        className="task-main"
                        onClick={() => onOpen(f.customerId)}
                      >
                        <strong>{f.type}</strong>
                        <span>
                          {
                            data.customers.find((c) => c.id === f.customerId)
                              ?.name
                          }{" "}
                          · {date(f.due)}
                        </span>
                      </button>
                      <button className="button" onClick={() => setTask(f)}>
                        Actualizează
                      </button>
                    </div>
                  ))}
                  {!tasks.length && <Empty>Nicio revenire în așteptare.</Empty>}
                </section>
              )}
              {tab === "activity" && (
                <section className="card employee-activity">
                  <div className="section-heading">
                    <h2>Activitate înregistrată</h2>
                    {controls}
                  </div>
                  {!valid ? null : q.isPending ? (
                    <p>Se încarcă activitatea…</p>
                  ) : q.error ? (
                    <p role="alert" className="error">
                      {q.error.message}
                    </p>
                  ) : (
                    <>
                      <p className="activity-summary">
                        {visits.length} vizite ·{" "}
                        {q.data?.events.filter(
                          (e) => e.employeeId === id && e.stage === "offer",
                        ).length ?? 0}{" "}
                        treceri la ofertă ·{" "}
                        {q.data?.events.filter(
                          (e) => e.employeeId === id && e.stage === "won",
                        ).length ?? 0}{" "}
                        câștiguri în interval
                      </p>
                      {[...visits]
                        .sort((a, b) => b.at.localeCompare(a.at))
                        .map((v) => (
                          <button
                            className="employee-activity-row"
                            key={v.id}
                            onClick={() => onOpen(v.customerId)}
                          >
                            <span className="activity-date">{date(v.at)}</span>
                            <span>
                              <strong>
                                {
                                  data.customers.find(
                                    (c) => c.id === v.customerId,
                                  )?.name
                                }
                              </strong>
                              <small>
                                {v.reason || "Vizită"}
                                {v.notes ? " · " + v.notes : ""}
                              </small>
                            </span>
                            <ArrowRight size={16} />
                          </button>
                        ))}
                      {!visits.length && (
                        <Empty>Nicio vizită în acest interval.</Empty>
                      )}
                    </>
                  )}
                </section>
              )}
            </>
          )}
        </div>
      </div>
      {task && (
        <FollowUpForm
          customerId={task.customerId}
          customerName={
            data.customers.find((c) => c.id === task.customerId)?.name ?? ""
          }
          user={user}
          users={members}
          followUp={task}
          onClose={() => setTask(null)}
        />
      )}
    </>
  );
}
