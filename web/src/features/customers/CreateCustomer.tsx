import { useMutation, useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight } from "lucide-react";
import { Modal } from "../../components/ui";
import { api, ApiError } from "../../api";
import {
  customerPageSchema,
  customerSchema,
  normalizePhone,
  type Customer,
} from "../../domain";
import { useDebounced } from "../../hooks";
const validPhone = (v: string) => /^\+[1-9]\d{7,14}$/.test(normalizePhone(v));
const createSchema = z.object({
  name: z.string().trim().max(120),
  phone: z
    .string()
    .min(8, "Introdu un număr de telefon valid.")
    .regex(/^[+\d\s()-]+$/, "Folosește doar cifre și prefixul țării.")
    .refine(
      validPhone,
      "Introdu un număr de telefon valid, cu prefix internațional sau 07…",
    ),
});
export default function CreateCustomer({
  initialPhone = "",
  onClose,
  onSaved,
  onOpenExisting,
}: {
  initialPhone?: string;
  onClose: () => void;
  onSaved: (c: Customer) => void;
  onOpenExisting?: (id: string) => void;
}) {
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    mode: "onTouched",
    defaultValues: { name: "", phone: initialPhone },
  });
  // The phone number identifies a customer; warn before creating another profile for it.
  const phone = useDebounced(form.watch("phone"), 300);
  const existing = useQuery({
    queryKey: ["customers", "phone", normalizePhone(phone)],
    queryFn: async () =>
      customerPageSchema.parse(
        await api(
          "customers?phone=" + encodeURIComponent(normalizePhone(phone)),
        ),
      ),
    enabled: validPhone(phone),
  });
  const matches = existing.data?.items ?? [];
  const save = useMutation({
    mutationFn: (values: z.infer<typeof createSchema>) =>
      api("customers", values),
    onSuccess: (v) => onSaved(customerSchema.parse(v)),
    onError: (e) => {
      // Show server validation next to the matching field.
      if (e instanceof ApiError)
        for (const field of ["name", "phone"] as const)
          if (e.fields[field])
            form.setError(field, { message: e.fields[field] });
    },
  });
  return (
    <Modal title="O relație nouă începe aici." onClose={onClose}>
      <form onSubmit={form.handleSubmit((v) => save.mutate(v))}>
        <label htmlFor="phone">Număr de telefon</label>
        <input
          id="phone"
          autoFocus
          required
          type="tel"
          autoComplete="tel"
          placeholder="0722 345 678"
          {...form.register("phone")}
        />
        {form.formState.errors.phone && (
          <p className="error">{form.formState.errors.phone.message}</p>
        )}
        {matches.length > 0 ? (
          <div className="duplicate-warning" role="status">
            <strong>
              {matches.length === 1
                ? "Acest număr aparține deja unui client."
                : `Acest număr aparține deja la ${matches.length} clienți.`}
            </strong>
            {matches.slice(0, 3).map((c) => (
              <button
                type="button"
                key={c.id}
                className="text-link"
                onClick={() => onOpenExisting?.(c.id)}
              >
                Deschide {c.name || c.phone}
              </button>
            ))}
            <span>
              Creează un profil nou doar dacă este altă persoană (de exemplu un
              număr de familie).
            </span>
          </div>
        ) : (
          <p className="form-hint">
            Poți folosi și formatul +40. Numărul identifică clientul.
          </p>
        )}
        <label htmlFor="name">Nume client</label>
        <p className="form-hint">
          Opțional. Poți continua doar cu numărul de telefon.
        </p>
        <input
          id="name"
          autoComplete="name"
          placeholder="Nume și prenume"
          {...form.register("name")}
        />
        {form.formState.errors.name && (
          <p className="error">{form.formState.errors.name.message}</p>
        )}
        {save.error &&
          !(
            save.error instanceof ApiError &&
            save.error.code === "VALIDATION_FAILED"
          ) && (
            <p className="error" role="alert">
              {save.error.message}
            </p>
          )}
        <button className="button primary full" disabled={save.isPending}>
          {save.isPending ? "Se salvează…" : "Adaugă clientul"}
          <ArrowRight size={18} />
        </button>
      </form>
    </Modal>
  );
}
