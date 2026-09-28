import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Check } from "lucide-react";
import { Avatar, Modal } from "../../components/ui";
import { api } from "../../api";
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
  const [ownership, setOwnership] = useState("keep");
  const [action, setAction] = useState("");
  const [due, setDue] = useState(today());
  const [reason, setReason] = useState("Service / suport");
  const [notes, setNotes] = useState("");
  const [product, setProduct] = useState("");
  const save = useMutation({
    mutationFn: () =>
      api("customers/" + customer.id + "/visits", {
        reason,
        steps: selected,
        notes,
        ownership,
        nextAction: action,
        due,
        product,
      }),
    onSuccess: onSaved,
  });
  return (
    <Modal title="O conversație de ținut minte." onClose={onClose}>
      <div className="visit-customer">
        <Avatar name={customer.name} small />
        <strong>{customer.name}</strong>
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
          {[
            "Service / suport",
            "Factură",
            "Abonament nou",
            "Reînnoire abonament",
            "Telefon mobil",
            "Internet",
            "TV",
            "Accesorii",
            "Întrebare despre contract",
            "Problemă tehnică",
            "Altele",
          ].map((r) => (
            <option key={r}>{r}</option>
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
        <div className="form-columns">
          <div>
            <label htmlFor="action">Următorul pas</label>
            <select
              id="action"
              value={action}
              onChange={(e) => setAction(e.target.value)}
            >
              <option value="">Fără acțiune</option>
              {[
                "Sună clientul",
                "Clientul revine",
                "Verifică eligibilitatea",
                "Pregătește oferta",
                "Discută reînnoirea",
                "Așteptăm clientul",
              ].map((a) => (
                <option key={a}>{a}</option>
              ))}
            </select>
          </div>
          {action && (
            <div>
              <label htmlFor="due">Data revenirii</label>
              <input
                id="due"
                type="date"
                required
                value={due}
                onChange={(e) => setDue(e.target.value)}
              />
            </div>
          )}
        </div>
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
          <p className="error" role="alert">
            {save.error.message}
          </p>
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
