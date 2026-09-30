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
    <span
      data-tone={(name.charCodeAt(0) || 0) % 5}
      className={"avatar " + (small ? "small" : "")}
    >
      {/^[+\d]/.test(name) ? "☎" : initials(name) || "☎"}
    </span>
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
  className = "",
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  className?: string;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialog?.showModal();
    return () => {
      dialog?.close();
      document.body.style.overflow = previousOverflow;
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={className}
      aria-labelledby={titleId}
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="modal-head">
        <div>
          <span className="eyebrow">BRIGHT SIGNA · RELAȚII CU CLIENȚII</span>
          <h2 id={titleId}>{title}</h2>
        </div>
        <button className="icon-button" aria-label="Închide" onClick={onClose}>
          <X />
        </button>
      </div>
      <div className="modal-body">{children}</div>
    </dialog>
  );
}
