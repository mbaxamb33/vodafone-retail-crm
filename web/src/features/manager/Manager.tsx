import { useState } from "react";
import { Link } from "react-router-dom";
import {
  ArrowRight,
  Clock,
  Users,
  TrendingUp,
  ChevronRight,
} from "lucide-react";
import { Empty, Avatar } from "../../components/ui";
import {
  today,
  date,
  stages,
  activeOpportunity,
  missingNextStep,
  staleOpportunity,
  type Workspace,
} from "../../domain";
import { useStoreReport } from "./reporting";
export default function Manager({
  data,
  onOpen,
}: {
  data: Workspace;
  onOpen: (id: string) => void;
}) {
  const { query: q, controls, valid } = useStoreReport();
  const [queue, setQueue] = useState("overdue");
  const open = data.opportunities.filter(activeOpportunity);
  const overdue = data.followUps
    .filter((f) => f.status !== "done" && f.due < today())
    .sort((a, b) => a.due.localeCompare(b.due));
  const stale = open.filter((o) => staleOpportunity(o));
  const missing = open.filter((o) => missingNextStep(o, data.followUps));
  const pool = data.customers.filter((c) => c.ownership === "pool");
  const queues = [
    ["overdue", "Reveniri restante", overdue.length],
    ["stale", "Oportunități blocate", stale.length],
    ["missing", "Fără pas următor", missing.length],
    ["pool", "De alocat", pool.length],
  ] as const;
  const rows =
    queue === "overdue"
      ? overdue.map((f) => ({
          id: f.id,
          customerId: f.customerId,
          owner: f.employeeId,
          title: f.type,
          detail: "Scadent " + date(f.due),
        }))
      : queue === "pool"
        ? pool.map((c) => ({
            id: c.id,
            customerId: c.id,
            owner: "",
            title: "Alege un responsabil",
            detail: "Portofoliul magazinului",
          }))
        : (queue === "stale" ? stale : missing).map((o) => ({
            id: o.id,
            customerId: o.customerId,
            owner: o.employeeId,
            title: o.product,
            detail:
              queue === "stale"
                ? "Fără schimbare de etapă de cel puțin 7 zile"
                : "Niciun follow-up legat de această oportunitate",
          }));
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">MAGAZIN · PRIVIRE DE ANSAMBLU</span>
          <h1>Ce are nevoie de atenția ta?</h1>
          <p>Prioritățile magazinului, înaintea cifrelor.</p>
        </div>
        <Link to="/team" className="button">
          Vezi echipa
          <ArrowRight size={17} />
        </Link>
      </div>
      <div className="stats-grid manager-current">
        {[
          {
            label: "Reveniri restante",
            count: overdue.length,
            icon: Clock,
            queue: "overdue",
          },
          {
            label: "Oportunități active",
            count: open.length,
            icon: TrendingUp,
            queue: "missing",
          },
          {
            label: "Clienți de alocat",
            count: pool.length,
            icon: Users,
            queue: "pool",
          },
        ].map((m) => (
          <button
            className="stat-card"
            key={m.label}
            onClick={() => setQueue(m.queue)}
          >
            <div>
              {m.label}
              <m.icon size={19} />
            </div>
            <strong>{m.count}</strong>
            <small>Situația curentă · întregul magazin</small>
          </button>
        ))}
      </div>
      <section className="card attention-center">
        <div className="section-heading">
          <div>
            <h2>Unde poți face diferența</h2>
            <p>Selectează un client pentru context și următorul pas.</p>
          </div>
        </div>
        <div className="tabs wrap-tabs">
          {queues.map(([id, label, count]) => (
            <button
              key={id}
              className={queue === id ? "active" : ""}
              onClick={() => setQueue(id)}
            >
              {label}
              <span className="count-pill">{count}</span>
            </button>
          ))}
        </div>
        {rows.slice(0, 10).map((row) => {
          const customer = data.customers.find((c) => c.id === row.customerId)!;
          return (
            <div className="attention-row" key={row.id}>
              <Avatar name={customer.name || customer.phone} />
              <button className="task-main" onClick={() => onOpen(customer.id)}>
                <strong>{customer.name || customer.phone}</strong>
                <span>{row.title}</span>
                <span>{row.detail}</span>
              </button>
              {row.owner && (
                <Link className="owner-link" to={"/team?employee=" + row.owner}>
                  {data.users.find((u) => u.id === row.owner)?.name}
                  <ChevronRight size={14} />
                </Link>
              )}
              <button
                className="icon-button"
                aria-label={"Deschide " + customer.name}
                onClick={() => onOpen(customer.id)}
              >
                <ArrowRight size={18} />
              </button>
            </div>
          );
        })}
        {!rows.length && <Empty>Nicio prioritate în această categorie.</Empty>}
        {rows.length > 10 && (
          <p className="form-hint">
            Primele 10 din {rows.length}. Consultă portofoliile echipei pentru
            lista completă.
          </p>
        )}
      </section>
      <section className="report-section">
        <div className="section-heading">
          <div>
            <span className="eyebrow">ACTIVITATE ÎN INTERVAL</span>
            <h2>Ce s-a întâmplat în magazin</h2>
            <p>
              Evenimente înregistrate în perioada selectată, ora Bucureștiului.
            </p>
          </div>
          {controls}
        </div>
        {!valid ? null : q.isPending ? (
          <div className="skeleton">
            <div />
          </div>
        ) : q.error ? (
          <div className="error-panel">
            <p className="error">{q.error.message}</p>
            <button className="button" onClick={() => q.refetch()}>
              Reîncearcă
            </button>
          </div>
        ) : (
          q.data && (
            <>
              <div className="stats-grid report-stats">
                {[
                  ["Vizite înregistrate", q.data.summary.visits],
                  ["Clienți noi", q.data.summary.newCustomers],
                  ["Treceri la ofertă", q.data.summary.offers],
                  ["Contracte câștigate", q.data.summary.contracts],
                ].map(([label, count]) => (
                  <div className="stat-card" key={label}>
                    <div>{label}</div>
                    <strong>{count}</strong>
                    <small>În intervalul selectat</small>
                  </div>
                ))}
              </div>
              <p className="report-note">
                Ofertele și contractele sunt numărate din schimbările de etapă
                înregistrate. O oportunitate poate ajunge la ofertă de mai multe
                ori.
              </p>
              <section className="card funnel">
                <div className="section-heading">
                  <div>
                    <h2>Cât de departe ajung conversațiile</h2>
                    <p>
                      Vizite după cel mai avansat pas atins. Procentul arată
                      trecerea față de nivelul anterior.
                    </p>
                  </div>
                </div>
                {q.data.funnel.map((level, i) => (
                  <div className="funnel-line" key={level.step}>
                    <span>{level.label}</span>
                    <div>
                      <i
                        style={{
                          width: `${q.data.summary.visits ? (level.count / q.data.summary.visits) * 100 : 0}%`,
                        }}
                      />
                    </div>
                    <strong>{level.count}</strong>
                    <small>
                      {i === 0
                        ? "toate vizitele"
                        : `${Math.round(level.rate * 100)}% din pasul anterior`}
                    </small>
                  </div>
                ))}
              </section>
              <section className="card">
                <div className="section-heading">
                  <div>
                    <h2>Echipa în interval</h2>
                    <p>Selectează un coleg pentru portofoliu și activitate.</p>
                  </div>
                </div>
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Coleg</th>
                        <th>Clienți serviți</th>
                        <th>Clienți noi</th>
                        <th>Oportunități noi</th>
                        <th>Oferte</th>
                        <th>Contracte</th>
                        <th>Restanțe acum</th>
                      </tr>
                    </thead>
                    <tbody>
                      {q.data.employees.map((e) => (
                        <tr key={e.employeeId}>
                          <td>
                            <Link
                              className="employee-link"
                              to={"/team?employee=" + e.employeeId}
                            >
                              <Avatar name={e.name} small />
                              {e.name}
                            </Link>
                          </td>
                          <td data-label="Clienți serviți">
                            {e.customersHandled}
                          </td>
                          <td data-label="Clienți noi">{e.newCustomers}</td>
                          <td data-label="Oportunități noi">
                            {e.opportunitiesCreated}
                          </td>
                          <td data-label="Oferte">{e.offers}</td>
                          <td data-label="Contracte">{e.contracts}</td>
                          <td data-label="Restanțe">{e.overdueFollowUps}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
              <section className="card step-incidence">
                <div className="section-heading">
                  <div>
                    <h2>Pași parcurși în vizite</h2>
                    <p>
                      Frecvență în vizite, nu rată de conversie. O vizită poate
                      include mai mulți pași.
                    </p>
                  </div>
                </div>
                {q.data.stepIncidence.map((step) => {
                  const total = q.data.summary.visits;
                  return (
                    <div className="incidence-row" key={step.step}>
                      <span>{step.label}</span>
                      <div className="incidence-track">
                        <i
                          style={{
                            width: `${total ? (step.count / total) * 100 : 0}%`,
                          }}
                        />
                      </div>
                      <strong>
                        {step.count} / {total}
                      </strong>
                      <small>
                        {total ? Math.round((step.count / total) * 100) : 0}%
                      </small>
                    </div>
                  );
                })}
              </section>
            </>
          )
        )}
      </section>
      <section className="card store-pipeline">
        <div className="section-heading">
          <div>
            <h2>Pipeline-ul curent</h2>
            <p>
              Oportunități deschise acum · independent de perioada raportului
            </p>
          </div>
          <Link className="text-link" to="/pipeline?scope=all">
            Deschide pipeline-ul
            <ArrowRight size={16} />
          </Link>
        </div>
        <div className="stage-totals">
          {Object.entries(stages)
            .filter(([id]) => !["won", "lost"].includes(id))
            .map(([id, label]) => (
              <Link to={"/pipeline?scope=all&stage=" + id} key={id}>
                <strong>{open.filter((o) => o.stage === id).length}</strong>
                <span>{label}</span>
              </Link>
            ))}
        </div>
      </section>
    </>
  );
}
