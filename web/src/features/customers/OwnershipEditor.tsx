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
  const [target, setTarget] = useState(customer.ownerId || customer.ownership);
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
  const canEdit =
    user.role === "manager" ||
    !customer.ownerId ||
    customer.ownerId === user.id;
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
              setTarget(customer.ownerId || customer.ownership);
              setEditing(true);
            }}
          >
            {customer.ownerId ? "Schimbă" : "Alocă un responsabil"}
          </button>
        )}
      </div>
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
            {users
              .filter((u) => user.role === "manager" || u.id === user.id)
              .map((u) => (
                <option key={u.id} value={u.id}>
                  {u.id === user.id ? "Îl iau eu — " : ""}
                  {u.name}
                </option>
              ))}
            <option value="pool">Îl dau magazinului</option>
            <option value="unassigned">Fără urmărire activă</option>
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
