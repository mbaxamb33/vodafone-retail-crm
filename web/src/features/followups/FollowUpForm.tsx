import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api";
import { Modal } from "../../components/ui";
import {
  today,
  followUpStatuses,
  type FollowUp,
  type User,
} from "../../domain";
export default function FollowUpForm({
  customerId,
  customerName,
  opportunityId,
  user,
  users,
  followUp,
  onClose,
}: {
  customerId: string;
  customerName: string;
  opportunityId?: string;
  user: User;
  users: User[];
  followUp?: FollowUp;
  onClose: () => void;
}) {
  const [type, setType] = useState(followUp?.type ?? "Sună clientul");
  const [due, setDue] = useState(followUp?.due ?? today());
  const [status, setStatus] = useState(followUp?.status ?? "open");
  const [employeeId, setEmployeeId] = useState(user.id);
  const qc = useQueryClient();
  const save = useMutation({
    mutationFn: () =>
      followUp
        ? api("follow-ups/" + followUp.id, { status, due }, "PATCH")
        : api("follow-ups", {
            customerId,
            opportunityId: opportunityId ?? "",
            employeeId,
            type,
            due,
          }),
    onSuccess: () => {
      void qc.invalidateQueries();
      onClose();
    },
  });
  return (
    <Modal
      title={
        followUp ? "Actualizează următorul pas" : "Stabilește următorul pas"
      }
      onClose={onClose}
    >
      <p>{customerName}</p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        {!followUp && (
          <>
            <label htmlFor="followup-type">Ce trebuie făcut?</label>
            <input
              id="followup-type"
              required
              maxLength={100}
              value={type}
              onChange={(e) => setType(e.target.value)}
            />
            <label htmlFor="followup-owner">Cine revine la client?</label>
            <select
              id="followup-owner"
              value={employeeId}
              onChange={(e) => setEmployeeId(e.target.value)}
            >
              {users
                .filter((u) => user.role === "manager" || u.id === user.id)
                .map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
            </select>
          </>
        )}
        {followUp && (
          <>
            <h3 className="form-summary">{followUp.type}</h3>
            <label htmlFor="followup-status">Rezultatul revenirii</label>
            <select
              id="followup-status"
              value={status}
              onChange={(e) => setStatus(e.target.value as FollowUp["status"])}
            >
              <option value="open">Reprogramează</option>
              <option value="waiting">{followUpStatuses.waiting}</option>
              <option value="unreachable">
                {followUpStatuses.unreachable}
              </option>
              <option value="done">{followUpStatuses.done}</option>
            </select>
          </>
        )}
        {status !== "done" && (
          <>
            <label htmlFor="followup-due">
              {followUp ? "Când revenim?" : "Data revenirii"}
            </label>
            <input
              id="followup-due"
              type="date"
              required
              value={due}
              onChange={(e) => setDue(e.target.value)}
            />
            <p className="form-hint">
              Chiar dacă așteptăm clientul, păstrăm o dată la care verificăm
              situația.
            </p>
          </>
        )}
        {save.error && (
          <p role="alert" className="error">
            {save.error.message}
          </p>
        )}
        <div className="action-row">
          <button className="button primary" disabled={save.isPending}>
            {save.isPending ? "Se salvează…" : "Salvează următorul pas"}
          </button>
          <button type="button" className="button" onClick={onClose}>
            Anulează
          </button>
        </div>
      </form>
    </Modal>
  );
}
