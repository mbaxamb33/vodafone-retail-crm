export function BrandMark({ className = "" }: { className?: string }) {
  return (
    <svg
      className={"brand-symbol " + className}
      viewBox="0 0 40 40"
      fill="none"
      aria-hidden="true"
    >
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M20 2 25.3 14.7 38 20 25.3 25.3 20 38 14.7 25.3 2 20 14.7 14.7 20 2Zm0 12-6 6 6 6 6-6-6-6Z"
      />
      <path
        fill="currentColor"
        d="m33 1 1.7 4.3L39 7l-4.3 1.7L33 13l-1.7-4.3L27 7l4.3-1.7L33 1Z"
      />
    </svg>
  );
}
export default function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <span className={"bright-brand " + (compact ? "compact" : "")}>
      <BrandMark />
      <span className="brand-wordmark">
        <strong>Bright</strong> Signa
      </span>
    </span>
  );
}
