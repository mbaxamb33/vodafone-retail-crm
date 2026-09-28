import { useEffect, useRef, useId, type ReactNode } from "react";
import { CheckCheck, X } from "lucide-react";
import { initials } from "../domain";
export function Avatar({
  name,
  small = false,
}: {
  name: string;
  small?: boolean;
}) {
  return (
    <span className={"avatar " + (small ? "small" : "")}>{initials(name)}</span>
  );
}
export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="empty">
      <CheckCheck size={30} />
      <h3>Ești la zi.</h3>
      <p>{children}</p>
    </div>
  );
}
export function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="modal-head">
        <div>
          <span className="eyebrow">VODAFONE · RELAȚII CU CLIENȚII</span>
          <h2 id={titleId}>{title}</h2>
        </div>
        <button className="icon-button" aria-label="Închide" onClick={onClose}>
          <X />
        </button>
      </div>
      {children}
    </dialog>
  );
}
