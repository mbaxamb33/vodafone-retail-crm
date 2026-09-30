import { customerLabel } from "../../domain";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { ArrowUpRight, CalendarDays, Plus } from "lucide-react";
import { api } from "../../api";
import {
  stages,
  date,
  activeOpportunity,
  nextOpportunityAction,
  missingNextStep,
  staleOpportunity,
  type Workspace,
  type User,
  type Opportunity,
  type FollowUp,
} from "../../domain";
import FollowUpForm from "../followups/FollowUpForm";
import { useNextStepLabel } from "../../hooks";
export default function Pipeline({
  data,
  user,
  onOpen,
  refresh,
}: {
  data: Workspace;
  user: User;
  onOpen: (id: string) => void;
  refresh: (s: string) => void;
}) {
  const [params, setParams] = useSearchParams();
  const stepLabel = useNextStepLabel();
  const owner =
    user.role === "manager" && params.get("scope") === "all" ? "all" : "me";
  const stage = params.get("stage") ?? "";
  const [attention, setAttention] = useState("all");
  const [action, setAction] = useState<{
    opportunity: Opportunity;
    followUp?: FollowUp;
  } | null>(null);
  const update = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: string }) =>
      api("opportunities/" + id, { stage }, "PATCH"),
    onSuccess: () => refresh("Oportunitatea a fost actualizată."),
  });
  const list = data.opportunities.filter(
    (o) =>
      (owner === "all" || o.employeeId === user.id) &&
      (attention === "all" ||
        (activeOpportunity(o) &&
          (attention === "missing"
            ? missingNextStep(o, data.followUps)
            : staleOpportunity(o)))),
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">POTENȚIAL ÎN FIECARE CONVERSAȚIE</span>
          <h1>Oportunități</h1>
          <p>Un responsabil și un pas următor pentru fiecare conversație.</p>
        </div>
        {user.role === "manager" && (
          <select
            aria-label="Portofoliu oportunități"
            value={owner}
            onChange={(e) => {
              const next = new URLSearchParams(params);
              next.set("scope", e.target.value);
              setParams(next);
            }}
          >
            <option value="me">Oportunitățile mele</option>
            <option value="all">Tot magazinul</option>
          </select>
        )}
      </div>
      <div className="pipeline-filters">
        <div className="tabs">
          {[
            ["all", "Toate"],
            ["missing", "Fără pas următor"],
            ["stale", "Fără progres de 7 zile"],
          ].map(([value, label]) => (
            <button
              key={value}
              className={attention === value ? "active" : ""}
              onClick={() => setAttention(value)}
            >
              {label}
            </button>
          ))}
        </div>
        {stage && (
          <button
            className="button"
            onClick={() => {
              const next = new URLSearchParams(params);
              next.delete("stage");
              setParams(next);
            }}
          >
            {stages[stage] ?? stage} · Arată toate etapele
          </button>
        )}
      </div>
      {update.error && (
        <p className="error" role="alert">
          {update.error.message}
        </p>
      )}
      <div className="mobile-stage-filter">
        <label htmlFor="mobile-stage">Etapa conversației</label>
        <select
          id="mobile-stage"
          value={stage}
          onChange={(e) => {
            const next = new URLSearchParams(params);
            if (e.target.value) next.set("stage", e.target.value);
            else next.delete("stage");
            setParams(next);
          }}
        >
          <option value="">Toate etapele</option>
          {Object.entries(stages).map(([id, label]) => (
            <option key={id} value={id}>
              {label} · {list.filter((o) => o.stage === id).length}
            </option>
          ))}
        </select>
        <span>
          {list.filter((o) => !stage || o.stage === stage).length} oportunități
        </span>
      </div>
      <div className="kanban" data-filtered={Boolean(stage)}>
        {Object.entries(stages)
          .filter(([id]) => !stage || id === stage)
          .map(([id, label]) => (
            <section
              key={id}
              className="kanban-column"
              data-empty={!list.some((o) => o.stage === id)}
            >
              <h3>
                <span
                  className={"stage-square " + (id === "offer" ? "s3" : "s1")}
                />
                {label}
                <span>{list.filter((o) => o.stage === id).length}</span>
              </h3>
              {list
                .filter((o) => o.stage === id)
                .map((o) => {
                  const c = data.customers.find((c) => c.id === o.customerId)!;
                  const next = nextOpportunityAction(o, data.followUps);
                  return (
                    <article className="opportunity-card" key={o.id}>
                      <span className="tag">{o.product}</span>
                      <button onClick={() => onOpen(c.id)}>
                        <h3>{c.name || c.phone}</h3>
                        <ArrowUpRight size={16} />
                      </button>
                      <p className="opportunity-owner">
                        {data.users.find((u) => u.id === o.employeeId)?.name} ·{" "}
                        {date(o.createdAt)}
                      </p>
                      {activeOpportunity(o) && (
                        <>
                          {staleOpportunity(o) && (
                            <p className="stale-label">
                              Fără progres de cel puțin 7 zile
                            </p>
                          )}
                          {next ? (
                            <div className="pipeline-next">
                              <CalendarDays size={15} />
                              <div>
                                <strong>{next.type}</strong>
                                <small>
                                  {date(next.due)} ·{" "}
                                  {
                                    data.users.find(
                                      (u) => u.id === next.employeeId,
                                    )?.name
                                  }
                                </small>
                              </div>
                              {(user.role === "manager" ||
                                next.employeeId === user.id) && (
                                <button
                                  aria-label={
                                    "Actualizează pasul pentru " +
                                    customerLabel(c)
                                  }
                                  onClick={() =>
                                    setAction({
                                      opportunity: o,
                                      followUp: next,
                                    })
                                  }
                                >
                                  <ArrowUpRight size={16} />
                                </button>
                              )}
                            </div>
                          ) : (
                            <button
                              className="missing-next-button"
                              onClick={() => setAction({ opportunity: o })}
                            >
                              <Plus size={15} />
                              {o.nextStep
                                ? `${stepLabel(o.nextStep)} · fără dată`
                                : "Stabilește pasul următor"}
                            </button>
                          )}
                        </>
                      )}
                      <select
                        aria-label={
                          "Etapă pentru " + customerLabel(c) + " — " + o.product
                        }
                        value={o.stage}
                        disabled={update.isPending || !activeOpportunity(o)}
                        onChange={(e) => {
                          const stage = e.target.value;
                          if (
                            ["lost", "won"].includes(stage) &&
                            !window.confirm(
                              `Confirmi închiderea oportunității ca ${stages[stage].toLowerCase()}?`,
                            )
                          )
                            return;
                          update.mutate({ id: o.id, stage });
                        }}
                      >
                        {Object.entries(stages).map(([v, l]) => (
                          <option key={v} value={v}>
                            {l}
                          </option>
                        ))}
                      </select>
                    </article>
                  );
                })}
              {!list.some((o) => o.stage === id) && (
                <p className="column-empty">
                  Nicio oportunitate în această categorie
                </p>
              )}
            </section>
          ))}
      </div>
      {!list.length && (
        <div className="pipeline-empty">
          Nicio oportunitate pentru filtrele selectate.
        </div>
      )}
      {action && (
        <FollowUpForm
          customerId={action.opportunity.customerId}
          customerName={customerLabel(
            data.customers.find(
              (c) => c.id === action.opportunity.customerId,
            ) ?? { name: "", phone: "Client" },
          )}
          opportunityId={action.opportunity.id}
          followUp={action.followUp}
          user={user}
          users={data.users}
          onClose={() => setAction(null)}
        />
      )}
    </>
  );
}
