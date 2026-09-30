import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { type Customer, type User } from "../../domain";
export default function OwnershipEditor({
  customer,
  user,
  users,
  onSaved,
}: {
  customer: Customer;
  user: User;
  users: User[];
  onSaved?: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [target, setTarget] = useState(customer.ownerId ? "pool" : user.id);
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: () =>
      api("customers/" + customer.id + "/ownership", {
        ownership: ["pool", "unassigned"].includes(target) ? target : "owned",
        ownerId: ["pool", "unassigned"].includes(target) ? "" : target,
      }),
    onSuccess: () => {
      void qc.invalidateQueries();
      setEditing(false);
      onSaved?.();
    },
  });
  // Customers are claimed only from the pool. Only the owner, or a manager, returns one.
  const canEdit =
    !customer.ownerId ||
    customer.ownerId === user.id ||
    user.role === "manager";
  const label = customer.ownerId
    ? (users.find((u) => u.id === customer.ownerId)?.name ??
      "Colegul responsabil")
    : customer.ownership === "pool"
      ? "Portofoliul magazinului"
      : "Fără urmărire activă";
  return (
    <div className="ownership-editor">
      <div className="section-heading">
        <div>
          <span className="eyebrow">RESPONSABILUL RELAȚIEI</span>
          <strong>{label}</strong>
        </div>
        {canEdit && !editing && (
          <button
            className="button subtle"
            onClick={() => {
              setTarget(customer.ownerId ? "pool" : user.id);
              setEditing(true);
            }}
          >
            {customer.ownerId ? "Îl dau magazinului" : "Îl iau eu"}
          </button>
        )}
      </div>
      {!canEdit && (
        <p className="form-hint">
          Doar responsabilul sau managerul poate returna acest client
          magazinului. Poți înregistra vizite fără să preiei relația.
        </p>
      )}
      {editing && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (
              customer.ownerId &&
              target !== customer.ownerId &&
              !window.confirm(
                "Confirmi schimbarea responsabilului? Follow-up-urile existente rămân la colegii care le au în grijă.",
              )
            )
              return;
            save.mutate();
          }}
        >
          <label htmlFor={"owner-" + customer.id}>Responsabil nou</label>
          <select
            id={"owner-" + customer.id}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
          >
            {customer.ownerId ? (
              <option value="pool">Îl dau magazinului</option>
            ) : (
              <option value={user.id}>Îl iau eu — {user.name}</option>
            )}
          </select>
          <p className="form-hint">
            Schimbarea responsabilului nu creează o vizită și nu transferă
            follow-up-urile existente.
          </p>
          {save.error && (
            <p role="alert" className="error">
              {save.error.message}
            </p>
          )}
          <div className="action-row">
            <button className="button primary" disabled={save.isPending}>
              {save.isPending ? "Se salvează…" : "Salvează responsabilul"}
            </button>
            <button
              className="button"
              type="button"
              disabled={save.isPending}
              onClick={() => setEditing(false)}
            >
              Anulează
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
