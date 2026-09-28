import { useMutation } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight } from "lucide-react";
import { Modal } from "../../components/ui";
import { api } from "../../api";
import { customerSchema, type Customer } from "../../domain";
const createSchema = z.object({
  name: z.string().trim().min(2, "Introdu cel puțin două caractere.").max(120),
  phone: z
    .string()
    .min(8, "Introdu un număr de telefon valid.")
    .regex(/^[+\d\s()-]+$/, "Folosește doar cifre și prefixul țării."),
});
export default function CreateCustomer({
  initialPhone = "",
  onClose,
  onSaved,
}: {
  initialPhone?: string;
  onClose: () => void;
  onSaved: (c: Customer) => void;
}) {
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    mode: "onTouched",
    defaultValues: { name: "", phone: initialPhone },
  });
  const save = useMutation({
    mutationFn: (values: z.infer<typeof createSchema>) =>
      api("customers", values),
    onSuccess: (v) => onSaved(customerSchema.parse(v)),
  });
  return (
    <Modal title="O relație nouă începe aici." onClose={onClose}>
      <form onSubmit={form.handleSubmit((v) => save.mutate(v))}>
        <label htmlFor="name">Nume client</label>
        <input
          id="name"
          autoComplete="name"
          autoFocus
          placeholder="Nume și prenume"
          {...form.register("name")}
        />
        {form.formState.errors.name && (
          <p className="error">{form.formState.errors.name.message}</p>
        )}
        <label htmlFor="phone">Număr de telefon</label>
        <input
          id="phone"
          type="tel"
          autoComplete="tel"
          placeholder="0722 345 678"
          {...form.register("phone")}
        />
        {form.formState.errors.phone && (
          <p className="error">{form.formState.errors.phone.message}</p>
        )}
        <p className="form-hint">
          Poți folosi și formatul +40. Numerele comune sunt permise; verifică
          profilurile existente înainte de a crea unul nou.
        </p>
        {save.error && (
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
