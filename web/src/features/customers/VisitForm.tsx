import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { Avatar, Modal } from "../../components/ui";
import { api, ApiError } from "../../api";
import { useCatalog } from "../../hooks";
import { steps, today, type Customer, type User } from "../../domain";
export default function VisitForm({
  customer,
  user,
  users,
  onClose,
  onSaved,
}: {
  customer: Customer;
  user: User;
  users: User[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [selected, setSelected] = useState<number[]>([0]);
  const [ownership, setOwnership] = useState<"keep" | "owned">("keep");
  const [action, setAction] = useState("none");
  const [actionDetails, setActionDetails] = useState("");
  const [agreedDate, setAgreedDate] = useState(false);
  const [resolutionType, setResolutionType] = useState<"invoice" | "other">(
    "invoice",
  );
  const [holder, setHolder] = useState<"holder" | "other">("holder");
  const [resolutionStatus, setResolutionStatus] = useState<
    "resolved" | "unresolved" | "pending"
  >("resolved");
  const [remind, setRemind] = useState(false);
  const [reminderDue, setReminderDue] = useState("");
  const [reminderNotes, setReminderNotes] = useState("");
  const unresolved =
    selected.includes(1) &&
    resolutionType === "other" &&
    resolutionStatus !== "resolved";

  const [due, setDue] = useState(today());
  const [reason, setReason] = useState("support");
  const [notes, setNotes] = useState("");
  const [product, setProduct] = useState("");
  const [category, setCategory] = useState("");
  const catalog = useCatalog();
  const save = useMutation({
    mutationFn: () =>
      api("customers/" + customer.id + "/visits", {
        reasonCode: reason,
        steps: selected,
        notes,
        ownership,
        nextAction: action,
        actionDetails: action === "other" ? actionDetails : "",
        agreedDate,
        due: agreedDate ? due : "",
        resolution: selected.includes(1)
          ? {
              type: resolutionType,
              ...(resolutionType === "invoice"
                ? { holder }
                : { status: resolutionStatus }),
            }
          : null,
        reminder:
          unresolved && remind
            ? { due: reminderDue, notes: reminderNotes }
            : null,
        opportunities: product.trim()
          ? [{ product: product.trim(), category }]
          : [],
      }),
    onSuccess: onSaved,
  });
  const fieldError = (f: string) =>
    save.error instanceof ApiError ? save.error.fields[f] : undefined;
  const fieldErrors =
    save.error instanceof ApiError ? Object.values(save.error.fields) : [];
  return (
    <Modal title="O conversație de ținut minte." onClose={onClose}>
      <div className="visit-customer">
        <Avatar name={customer.name || customer.phone} small />
        <strong>{customer.name || customer.phone}</strong>
        <span>{customer.phone}</span>
      </div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <div className="visit-ownership-context">
          <strong>
            Responsabil:{" "}
            {customer.ownerId
              ? users.find((u) => u.id === customer.ownerId)?.name
              : customer.ownership === "pool"
                ? "Magazin"
                : "Fără urmărire activă"}
          </strong>
          <p className="form-hint">
            Poți înregistra această vizită fără să schimbi responsabilul
            relației.
          </p>
          {!customer.ownerId && (
            <label className="check-label">
              <input
                type="checkbox"
                checked={ownership === "owned"}
                onChange={(e) =>
                  setOwnership(e.target.checked ? "owned" : "keep")
                }
              />
              Îl iau eu în portofoliu ({user.name})
            </label>
          )}
        </div>
        <label htmlFor="reason">Motivul vizitei</label>
        <select
          id="reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        >
          <option value="">Fără motiv anume</option>
          {catalog.data?.visitReasons.map((r) => (
            <option key={r.code} value={r.code}>
              {r.label}
            </option>
          ))}
        </select>
        <label>Ce pași ai realizat?</label>
        <p className="form-hint">
          Selectează doar pașii parcurși în această vizită.
        </p>
        <div className="journey">
          {steps.map((s, i) => (
            <button
              type="button"
              key={s}
              aria-label={s}
              aria-pressed={selected.includes(i)}
              className={selected.includes(i) ? "selected" : ""}
              onClick={() =>
                setSelected(
                  selected.includes(i)
                    ? selected.filter((n) => n !== i)
                    : [...selected, i],
                )
              }
            >
              <span>{selected.includes(i) ? <Check size={15} /> : i + 1}</span>
              {s}
            </button>
          ))}
        </div>
        {selected.includes(1) && (
          <section className="visit-detail-panel">
            <h3>Rezolvarea solicitării</h3>
            <label htmlFor="resolution-type">Tipul solicitării</label>
            <select
              id="resolution-type"
              value={resolutionType}
              onChange={(e) =>
                setResolutionType(e.target.value as typeof resolutionType)
              }
            >
              <option value="invoice">Încasare factură</option>
              <option value="other">Alte solicitări</option>
            </select>
            {resolutionType === "invoice" ? (
              <>
                <label htmlFor="holder">Cine a venit?</label>
                <select
                  id="holder"
                  value={holder}
                  onChange={(e) => setHolder(e.target.value as typeof holder)}
                >
                  <option value="holder">Titular</option>
                  <option value="other">Netitular</option>
                </select>
              </>
            ) : (
              <>
                <label htmlFor="resolution-status">Starea solicitării</label>
                <select
                  id="resolution-status"
                  value={resolutionStatus}
                  onChange={(e) =>
                    setResolutionStatus(
                      e.target.value as typeof resolutionStatus,
                    )
                  }
                >
                  <option value="resolved">Rezolvat</option>
                  <option value="unresolved">Nerezolvat</option>
                  <option value="pending">
                    În așteptare · are caz deschis
                  </option>
                </select>
              </>
            )}
            {unresolved && (
              <>
                <label className="check-label">
                  <input
                    type="checkbox"
                    checked={remind}
                    onChange={(e) => setRemind(e.target.checked)}
                  />
                  Amintește-mi să verific rezolvarea
                </label>
                <p className="form-hint">
                  Reminder intern pentru tine. Nu presupune o dată convenită cu
                  clientul.
                </p>
                {remind && (
                  <>
                    <label htmlFor="reminder-due">
                      Când verifici problema?
                    </label>
                    <input
                      id="reminder-due"
                      type="date"
                      min={today()}
                      required
                      value={reminderDue}
                      onChange={(e) => setReminderDue(e.target.value)}
                    />
                    <label htmlFor="reminder-notes">
                      Ce trebuie verificat?
                    </label>
                    <input
                      id="reminder-notes"
                      required
                      maxLength={500}
                      value={reminderNotes}
                      onChange={(e) => setReminderNotes(e.target.value)}
                      placeholder="Ex. S-a rezolvat cazul de facturare?"
                    />
                  </>
                )}
              </>
            )}
          </section>
        )}
        <label htmlFor="action">Următorul pas stabilit cu clientul</label>
        <select
          id="action"
          value={action}
          onChange={(e) => setAction(e.target.value)}
        >
          {(catalog.data?.nextActions.length
            ? catalog.data.nextActions
            : [{ code: "none", label: "Nimic" }]
          ).map((a) => (
            <option key={a.code} value={a.code}>
              {a.label}
            </option>
          ))}
        </select>
        {action === "other" && (
          <>
            <label htmlFor="action-details">Ce ați stabilit?</label>
            <input
              id="action-details"
              required
              maxLength={500}
              value={actionDetails}
              onChange={(e) => setActionDetails(e.target.value)}
            />
          </>
        )}
        {action === "keep_in_touch" && (
          <p className="consent-note">
            Prin această alegere consemnezi că persoana și-a exprimat acordul să
            rămâneți în contact.
          </p>
        )}
        <section className="visit-detail-panel">
          <label className="check-label">
            <input
              type="checkbox"
              checked={agreedDate}
              onChange={(e) => setAgreedDate(e.target.checked)}
            />
            Am stabilit o dată împreună cu clientul
          </label>
          {agreedDate ? (
            <>
              <label htmlFor="due">Data revenirii</label>
              <input
                id="due"
                type="date"
                min={today()}
                required
                value={due}
                onChange={(e) => setDue(e.target.value)}
              />
              {fieldError("due") && (
                <p className="error">{fieldError("due")}</p>
              )}
            </>
          ) : (
            <p className="form-hint">
              Fără dată stabilită. Oportunitatea și următorul pas pot fi
              păstrate fără programare.
            </p>
          )}
        </section>
        <label htmlFor="product">
          Oportunitate nouă <span className="optional">opțional</span>
        </label>
        <input
          id="product"
          maxLength={120}
          placeholder="Ex. Reînnoire Red Unlimited"
          value={product}
          onChange={(e) => setProduct(e.target.value)}
        />
        {product.trim() && (
          <select
            aria-label="Categoria oportunității"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            <option value="">Fără categorie</option>
            {catalog.data?.productCategories.map((c) => (
              <option key={c.code} value={c.code}>
                {c.label}
              </option>
            ))}
          </select>
        )}
        <label htmlFor="notes">
          Ce ar trebui să ținem minte?{" "}
          <span className="optional">opțional</span>
        </label>
        <textarea
          id="notes"
          rows={3}
          maxLength={4000}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          placeholder="Context util pentru următoarea conversație…"
        />
        <p className="form-hint">
          Păstrează doar detaliile necesare. Evită datele personale sensibile.
        </p>
        {save.error && (
          <div className="error" role="alert">
            <p>{save.error.message}</p>
            {fieldErrors.length > 0 && (
              <ul>
                {fieldErrors.map((m) => (
                  <li key={m}>{m}</li>
                ))}
              </ul>
            )}
          </div>
        )}
        <button
          className="button primary full"
          disabled={!selected.length || save.isPending}
        >
          {save.isPending ? "Se salvează…" : "Salvează vizita"}
          <Check size={18} />
        </button>
      </form>
    </Modal>
  );
}
