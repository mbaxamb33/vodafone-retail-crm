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
import { useEmployeeActivity, useStoreReport } from "./reporting";
import { useCustomerPages } from "../../hooks";
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
  const { query: q, controls, valid, start, end } = useStoreReport();
  const members = data.users;
  const id = params.get("employee") ?? members[0]?.id;
  const employee = members.find((u) => u.id === id);
  const portfolio = useCustomerPages({ owner: id ?? "", sort: "name" });
  const customers = portfolio.data?.pages.flatMap((p) => p.items) ?? [];
  const activity = useEmployeeActivity(
    id,
    start,
    end,
    tab === "activity" && valid,
  );
  const reportFor = (userId: string) =>
    q.data?.employees.find((e) => e.employeeId === userId);
  const opportunities = data.opportunities.filter(
    (o) => o.employeeId === id && activeOpportunity(o),
  );
  const tasks = data.followUps
    .filter((f) => f.employeeId === id && f.status !== "done")
    .sort((a, b) => a.due.localeCompare(b.due));
  const visits = activity.data?.items ?? [];
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
                    {reportFor(member.id)?.portfolioCustomers ?? "…"} clienți
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
                    <strong>{portfolio.data?.pages[0]?.total ?? "…"}</strong>
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
                  {portfolio.isSuccess && !customers.length && (
                    <Empty>Acest coleg nu are clienți alocați.</Empty>
                  )}
                  {portfolio.hasNextPage && (
                    <button
                      className="button"
                      disabled={portfolio.isFetchingNextPage}
                      onClick={() => portfolio.fetchNextPage()}
                    >
                      Arată mai mulți
                    </button>
                  )}
                  {selectedCustomer && (
                    <OwnershipEditor
                      key={selectedCustomer.id}
                      customer={
                        customers.find((c) => c.id === selectedCustomer.id) ??
                        selectedCustomer
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
                  {!valid ? null : activity.isPending ? (
                    <p>Se încarcă activitatea…</p>
                  ) : activity.error ? (
                    <p role="alert" className="error">
                      {activity.error.message}
                    </p>
                  ) : (
                    <>
                      <p className="activity-summary">
                        {activity.data.summary.visits} vizite ·{" "}
                        {activity.data.summary.offers} treceri la ofertă ·{" "}
                        {activity.data.summary.contracts} câștiguri în interval
                      </p>
                      {activity.data.total > visits.length && (
                        <p className="form-hint">
                          Ultimele {visits.length} din {activity.data.total}{" "}
                          vizite.
                        </p>
                      )}
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
                                  activity.data.customers.find(
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
